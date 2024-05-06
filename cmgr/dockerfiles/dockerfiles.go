package dockerfiles

import (
	"embed"
)

func Get(challengeType string) ([]byte, error) {
	return dockerfiles.ReadFile(challengeType + ".Dockerfile")
}

//go:embed *.Dockerfile
var dockerfiles embed.FS
