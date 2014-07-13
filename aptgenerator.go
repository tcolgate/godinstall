package main

import (
	"errors"
	"io"
	"os"
	"os/exec"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// Interface for any Apt repository generator
type AptGenerator interface {
	Regenerate() error // Regenerate the apt archive
}

// An AptGenerator that uses the  apt-ftparchive from apt-utils
type aptFtpArchiveGenerator struct {
	Repo          *aptRepo        // The repo to update
	AftpPath      *string         // Path to the apt-ftp-archive binary
	AftpConfig    *string         // Path to the config for apt-ftp-archive
	ReleaseConfig *string         // Path to the release generation config file
	PrivRing      openpgp.KeyRing // Private keyring cotaining singing key
	SignerId      *openpgp.Entity // The key to sign release file with
}

// Create a new AptGenerator that uses apt-ftparchive
func NewAptFtpArchiveGenerator(
	repo *aptRepo,
	aftpPath *string,
	aftpConfig *string,
	releaseConfig *string,
	privRing openpgp.KeyRing,
	signerId *openpgp.Entity,
) AptGenerator {
	return &aptFtpArchiveGenerator{
		repo,
		aftpPath,
		aftpConfig,
		releaseConfig,
		privRing,
		signerId,
	}
}

// Run apt-ftparchive with the user provided configs to regenerate
// the archive stored int the provided AptRepo.
func (a *aptFtpArchiveGenerator) Regenerate() (err error) {
	err = exec.Command(*a.AftpPath, "generate", *a.AftpConfig).Run()
	if err != nil {
		if !err.(*exec.ExitError).Success() {
			return errors.New("apt-ftparchive failed, " + err.Error())
		}
	}

	if *a.ReleaseConfig != "" {
		// Generate the Releases and InReleases file
		releaseBase, _ := a.Repo.FindReleaseBase()
		releaseFilename := releaseBase + "/Release"

		releaseWriter, err := os.Create(releaseFilename)
		defer releaseWriter.Close()

		if err != nil {
			return errors.New("Error creating release file, " + err.Error())
		}

		cmd := exec.Command(*a.AftpPath, "-c", *a.ReleaseConfig, "release", releaseBase)
		releaseReader, _ := cmd.StdoutPipe()
		cmd.Start()
		io.Copy(releaseWriter, releaseReader)

		err = cmd.Wait()
		if err != nil {
			if !err.(*exec.ExitError).Success() {
				return errors.New("apt-ftparchive release generation failed, " + err.Error())
			}
			return err
		}

		if a.SignerId != nil {
			rereadRelease, err := os.Open(releaseFilename)
			defer rereadRelease.Close()

			releaseSignatureWriter, err := os.Create(releaseBase + "/Release.gpg")
			if err != nil {
				return errors.New("Error creating release signature file, " + err.Error())
			}
			defer releaseSignatureWriter.Close()

			err = openpgp.ArmoredDetachSign(releaseSignatureWriter, a.SignerId, rereadRelease, nil)
			if err != nil {
				return errors.New("Detached Sign failed, , " + err.Error())
			}

			rereadRelease2, err := os.Open(releaseFilename)
			defer rereadRelease2.Close()

			inReleaseSignatureWriter, err := os.Create(releaseBase + "/InRelease")
			if err != nil {
				return errors.New("Error creating InRelease file, " + err.Error())
			}
			inReleaseWriter, err := clearsign.Encode(inReleaseSignatureWriter, a.SignerId.PrivateKey, nil)
			if err != nil {
				return errors.New("Error InRelease clear-signer, " + err.Error())
			}
			io.Copy(inReleaseWriter, rereadRelease2)
			inReleaseWriter.Close()
		}
	}
	return err
}

// Mock for testing the apt generation interface
type aptFtpGeneratorMock struct {
}

func NewAptFtpGeneraterMock() AptGenerator {
	newgen := aptFtpGeneratorMock{}
	return newgen
}

func (aptFtpGeneratorMock) Regenerate() (err error) {
	return
}
