package cloud

import (
	"crypto/rand"
	goRSA "crypto/rsa"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"

	"github.com/quilt/quilt/connection/credentials/tls/rsa"
	"github.com/quilt/quilt/db"
)

// Test the success path when generating and installing credentials on a new
// machine.
func TestSyncCredentials(t *testing.T) {
	key, err := goRSA.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	expSigner, err := ssh.NewSignerFromKey(key)
	assert.NoError(t, err)

	expHost := "8.8.8.8"
	mockFs := afero.NewMemMapFs()

	getSftpFs = func(host string, signer ssh.Signer) (sftpFs, error) {
		assert.Equal(t, expSigner, signer)
		assert.Equal(t, expHost, host)
		return mockSFTPFs{mockFs}, nil
	}

	ca, err := rsa.NewCertificateAuthority()
	assert.NoError(t, err)

	credentialedMachines := map[string]struct{}{}
	syncCredentialsOnce("out", expSigner, ca,
		[]db.Machine{{PublicIP: expHost}}, credentialedMachines)
	assert.Len(t, credentialedMachines, 1)

	aferoFs := afero.Afero{Fs: mockFs}
	certBytes, err := aferoFs.ReadFile("out/quilt.crt")
	assert.NoError(t, err)
	assert.NotEmpty(t, certBytes)

	keyBytes, err := aferoFs.ReadFile("out/quilt.key")
	assert.NoError(t, err)
	assert.NotEmpty(t, keyBytes)

	caBytes, err := aferoFs.ReadFile("out/certificate_authority.crt")
	assert.NoError(t, err)
	assert.NotEmpty(t, caBytes)
}

func TestSyncCredentialsSkip(t *testing.T) {
	ca, err := rsa.NewCertificateAuthority()
	assert.NoError(t, err)

	// Test that we skip machines that have not booted yet.
	credentialedMachines := map[string]struct{}{}
	syncCredentialsOnce("", nil, ca,
		[]db.Machine{{Role: db.Worker}}, credentialedMachines)
	assert.Empty(t, credentialedMachines, 0)

	// Test that we skip machines that have already been setup.
	credentialedMachines = map[string]struct{}{
		"8.8.8.8": {},
	}
	syncCredentialsOnce("", nil, ca, []db.Machine{
		{Role: db.Worker, PublicIP: "8.8.8.8"},
	}, credentialedMachines)
	assert.Len(t, credentialedMachines, 1)

	// Test that if we fail to get an SFTP client, we bail.
	getSftpFs = func(host string, _ ssh.Signer) (sftpFs, error) {
		return nil, assert.AnError
	}
	credentialedMachines = map[string]struct{}{}
	syncCredentialsOnce("", nil, ca, []db.Machine{
		{Role: db.Worker, PublicIP: "8.8.8.8"},
	}, credentialedMachines)
	assert.Empty(t, credentialedMachines)
}

type mockSFTPFs struct {
	afero.Fs
}

func (fs mockSFTPFs) Close() error {
	return nil
}
