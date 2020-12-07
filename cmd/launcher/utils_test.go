package main

import (
	"bytes"
	"github.com/mholt/archiver"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"testing"
)

func TestFoo(t *testing.T) {
	sourceDir := "/disco-grande/go/src/github.com/kfsoftware/externalbuilder/examples"
	zipFile := "/disco-grande/go/src/github.com/kfsoftware/externalbuilder/tmp/example.tar"
	err := archiver.Archive([]string{sourceDir}, zipFile)
	assert.NoError(t, err)
	var buf bytes.Buffer
	err = compress(sourceDir, &buf)
	assert.NoError(t, err)
	err = ioutil.WriteFile(zipFile, buf.Bytes(), 0777)
	assert.NoError(t, err)

}

func TestGetBuildID(t *testing.T) {
	folder := "/tmp/fabric-ip-register-cc-ea2aaf81f5d991a883563c349e793ef0a642d577ceebb9a6778832c7fca23882425116466/bld"
	buildID, err := getBuildID(folder)
	assert.NoError(t, err)
	assert.Len(t, buildID, 10)
}
