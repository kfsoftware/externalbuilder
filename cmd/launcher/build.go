package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	cpy "github.com/otiai10/copy"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Build builds a chaincode on Kubernetes
func Build(ctx context.Context, cfg Config) error {
	log.Println("Procedure: build")

	if len(os.Args) != 4 {
		return errors.New("build requires exactly three arguments")
	}

	sourceDir := os.Args[1]
	metadataDir := os.Args[2]
	outputDir := os.Args[3]
	log.Printf("Source dir=%s", sourceDir)
	log.Printf("Metadata dir=%s", metadataDir)
	log.Printf("Output dir=%s", outputDir)
	for _, envKey := range os.Environ() {
		log.Printf("%s=%s", envKey, os.Getenv(envKey))
	}
	buildInfoFile := filepath.Join(outputDir, "k8scc_buildinfo.json")

	// Get metadata
	metadata, err := getMetadata(metadataDir)
	if err != nil {
		return errors.Wrap(err, "getting metadata for chaincode")
	}
	// /tmp/fabric-fabcar_1-1860815d78bd593aed9728d27eb8bb8c180b7e7e9918057eecb0cf6e4f38223d930256401/src
	buildID, err := getBuildID(sourceDir)
	if err != nil {
		return errors.Wrap(err, "getting buildid for chaincode")
	}
	var buf bytes.Buffer
	err = compress(sourceDir, &buf)
	//err = tarDirectory(sourceDir, chaincodeSourceZIP)
	if err != nil {
		return errors.Wrap(err, "creating the tar")
	}
	log.Printf("Tar created ")
	fileServerURL := getFileServerURL()
	basePathURL := fmt.Sprintf("%s/%s", fileServerURL, buildID)
	postURL := fmt.Sprintf("%s/chaincode-source.tar", basePathURL)
	log.Printf("Post URL=%s", postURL)
	resp, err := http.Post(
		postURL,
		"application/octet-stream",
		&buf,
	)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("Received %d code from server", resp.StatusCode)
	}
	log.Printf("File uploaded %d", resp.StatusCode)
	// Create builder Pod
	pod, err := createBuilderJob(ctx, cfg, metadata, basePathURL)
	if err != nil {
		return errors.Wrap(err, "creating builder pod")
	}

	// Watch builder Pod for completion or failure
	podSucceeded, err := watchPodUntilCompletion(ctx, pod)
	if err != nil {
		return errors.Wrap(err, "watching builder pod")
	}

	if !podSucceeded {
		return fmt.Errorf("build of Chaincode %s in Pod %s failed", metadata.Label, pod.Name)
	}
	transferSrcMeta := filepath.Join(sourceDir, "META-INF")

	// Copy META-INF, if available
	if _, err := os.Stat(transferSrcMeta); !os.IsNotExist(err) {
		err = cpy.Copy(transferSrcMeta, outputDir)
		if err != nil {
			return errors.Wrap(err, "copy META-INF to output dir")
		}
	}

	// Create build information
	buildInformation := BuildInformation{
		Image:    pod.Spec.InitContainers[2].Image,
		Platform: metadata.Type,
	}

	bi, err := json.Marshal(buildInformation)
	if err != nil {
		return errors.Wrap(err, "marshaling BuildInformation")
	}
	log.Printf("Metadata=%s", string(bi))

	err = ioutil.WriteFile(buildInfoFile, bi, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "writing BuildInformation")
	}

	err = os.Chmod(buildInfoFile, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "changing permissions of BuildInformation")
	}
	cleanupPodSilent(pod)
	return nil
}

func compress(src string, buf io.Writer) error {
	// tar > gzip > buf
	tw := tar.NewWriter(buf)

	// walk through every file in the folder
	filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		// generate tar header
		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		// must provide real name
		// (see https://golang.org/src/archive/tar/common.go?#L626)
		relname, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		if relname == "." {
			return nil
		}
		header.Name = filepath.ToSlash(relname)

		// write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		// if not a dir, write file content
		if !fi.IsDir() {
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, data); err != nil {
				return err
			}
		}
		return nil
	})

	// produce tar
	if err := tw.Close(); err != nil {
		return err
	}
	//
	return nil
}

func createBuilderJob(ctx context.Context, cfg Config, metadata *ChaincodeMetadata, basePathURL string) (*apiv1.Pod, error) {
	// Setup kubernetes client
	clientset, err := getKubernetesClientset()
	if err != nil {
		return nil, errors.Wrap(err, "getting kubernetes clientset")
	}

	// Get builder image
	image, ok := cfg.Images[strings.ToLower(metadata.Type)]
	if !ok {
		return nil, fmt.Errorf("no builder image available for %q", metadata.Type)
	}

	initImage := "dviejo/fabric-init:amd64-2.2.0"

	// Get platform informations from hyperledger
	plt := GetPlatform(metadata.Type)
	if plt == nil {
		return nil, fmt.Errorf("platform %q not supported by Hyperledger Fabric", metadata.Type)
	}

	buildOpts, err := plt.DockerBuildOptions(metadata.Path)
	if err != nil {
		return nil, errors.Wrap(err, "getting build options for platform")
	}

	envvars := []apiv1.EnvVar{}
	for _, env := range buildOpts.Env {
		s := strings.SplitN(env, "=", 2)
		envvars = append(envvars, apiv1.EnvVar{
			Name:  s[0],
			Value: s[1],
		})
	}
	for _, envItem := range cfg.Builder.Env {
		envvars = append(envvars, apiv1.EnvVar{
			Name:  envItem.Name,
			Value: envItem.Value,
		})
	}
	// Get peer Pod
	myself, _ := os.Hostname()
	myselfPod, err := clientset.CoreV1().Pods(cfg.Namespace).Get(ctx, myself, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "getting myself Pod")
	}

	// Set resources
	limits := apiv1.ResourceList{}
	if limit := cfg.Builder.Resources.LimitMemory; limit != "" {
		limits["memory"] = resource.MustParse(limit)
	}
	if limit := cfg.Builder.Resources.LimitCPU; limit != "" {
		limits["cpu"] = resource.MustParse(limit)
	}
	requests := apiv1.ResourceList{}
	if request := cfg.Builder.Resources.RequestsMemory; request != "" {
		requests["memory"] = resource.MustParse(request)
	}
	if request := cfg.Builder.Resources.RequestsCPU; request != "" {
		requests["cpu"] = resource.MustParse(request)
	}
	mounts := []apiv1.VolumeMount{
		{
			Name:      "chaincode",
			MountPath: "/chaincode",
		},
	}

	// Pod
	podname := fmt.Sprintf("%s-ccbuild-%s", myself, metadata.MetadataID)
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podname,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "v1",
					Kind:               "Pod",
					Name:               myselfPod.Name,
					UID:                myselfPod.UID,
					BlockOwnerDeletion: BoolRef(true),
				},
			},
			Labels: map[string]string{
				"externalcc-type": "builder",
			},
		},
		Spec: apiv1.PodSpec{
			InitContainers: []apiv1.Container{
				// setup chaincode volume
				{
					Image:   initImage,
					Name:    "setup-chaincode-volume",
					Command: []string{"/bin/bash"},
					Args: []string{
						`-c`,
						`mkdir -p /chaincode/input /chaincode/output && chmod 777 /chaincode/input /chaincode/output`,
					},
					VolumeMounts: mounts,
				},
				// download chaincode source
				{
					Image:   initImage,
					Name:    "download-chaincode-source",
					Command: []string{"/bin/bash"},
					Args: []string{
						"-c",
						fmt.Sprintf(`curl -s -o- -L '%s/chaincode-source.tar' | tar -C /chaincode/input -xvf - && chmod -R 777 /chaincode/input`, basePathURL),
					},
					VolumeMounts: mounts,
				},
				// build container
				{
					Name:            "builder",
					Image:           image,
					ImagePullPolicy: apiv1.PullIfNotPresent,
					Command: []string{
						"/bin/sh",
					},
					Args: []string{
						"-c", buildOpts.Cmd,
					},
					Env:          envvars,
					Resources:    apiv1.ResourceRequirements{Limits: limits, Requests: requests},
					VolumeMounts: mounts,
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            "upload-chaincode-output",
					Image:           initImage,
					ImagePullPolicy: apiv1.PullIfNotPresent,
					VolumeMounts:    mounts,
					Command:         []string{"/bin/bash"},
					Args: []string{
						"-c",
						//"sleep 600000",
						fmt.Sprintf(
							`
cp -r ./chaincode/input/META-INF ./chaincode/output/ || echo "META-INF doesn't exist" &&
cd /chaincode/output &&
tar cvf /chaincode/output.tar $(ls -A) &&
curl -X POST -s --upload-file /chaincode/output.tar '%s/chaincode-output.tar'`,
							basePathURL,
						),
					},
				},
			},
			EnableServiceLinks: BoolRef(false),
			RestartPolicy:      apiv1.RestartPolicyNever,
			Volumes: []apiv1.Volume{
				{
					Name: "chaincode",
				},
			},
		},
	}

	return clientset.CoreV1().Pods(cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
}
