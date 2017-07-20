package cloudcfg

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/version"

	log "github.com/Sirupsen/logrus"
)

const (
	quiltImage = "hantaowang/quilt"
)

// Allow mocking out for the unit tests.
var ver = version.Version

// Ubuntu generates a cloud config file for the Ubuntu operating system with the
// corresponding `version`.
func Ubuntu(keys []string, role db.Role) string {
	t := template.Must(template.New("cloudConfig").Parse(cfgTemplate))

	img := fmt.Sprintf("%s:%s", quiltImage, ver)

	var cloudConfigBytes bytes.Buffer
	err := t.Execute(&cloudConfigBytes, struct {
		QuiltImage    string
		UbuntuVersion string
		SSHKeys       string
		Role          string
		LogLevel      string
	}{
		QuiltImage:    img,
		UbuntuVersion: "xenial",
		SSHKeys:       strings.Join(keys, "\n"),
		Role:          string(role),
		LogLevel:      log.GetLevel().String(),
	})
	if err != nil {
		panic(err)
	}

	return cloudConfigBytes.String()
}
