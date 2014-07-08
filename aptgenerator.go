package main

import (
	"errors"
	"io"
	"os"
	"os/exec"

	"code.google.com/p/go.crypto/openpgp"
)

// Interface for any Apt archive generator
type AptGenerator interface {
	Regenerate() error // Regenerate the apt archive
}

// Runs apt-ftparchive
type aptFtpArchiveGenerator struct {
	Repo          *aptRepo
	AftpPath      *string
	AftpConfig    *string
	ReleaseConfig *string
	TmpDir        *string
}

func NewAptFtpArchiveGenerator(
	repo *aptRepo,
	aftpPath *string,
	aftpConfig *string,
	releaseConfig *string,
	tmpDir *string,
) AptGenerator {
	return &aptFtpArchiveGenerator{
		repo,
		aftpPath,
		aftpConfig,
		releaseConfig,
		tmpDir,
	}
}

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
				return
			}
		}
	}
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
