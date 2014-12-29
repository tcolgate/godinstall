package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
)

type request struct {
	req *http.Request
	err error
}

type response struct {
	Message struct {
		Complete  bool
		Expecting map[string]struct {
			Received bool
		}
	}
}

// Streams upload directly from file -> mime/multipart -> pipe -> http-request
func streamingUploadFile(res chan request, uri, paramName, path string) {
	file, err := os.Open(path)
	if err != nil {
		res <- request{nil, err}
		return
	}
	defer file.Close()

	r, w := io.Pipe()
	defer w.Close()

	req, err := http.NewRequest("POST", uri, r)
	if err != nil {
		res <- request{nil, err}
		return
	}

	writer := multipart.NewWriter(w)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res <- request{req, nil}

	part, err := writer.CreateFormFile(paramName, filepath.Base(path))
	if err != nil {
		log.Fatal(err)
		return
	}
	_, err = io.Copy(part, file)
	if err != nil {
		log.Fatal(err)
		return
	}

	err = writer.Close()
	if err != nil {
		log.Fatal(err)
		return
	}
}

// Creates a new file upload http request with optional extra params
func newfileUploadRequest(uri string, paramName, path string) (*http.Request, error) {
	req := make(chan request)
	go streamingUploadFile(req, uri, paramName, path)

	resp := <-req
	return resp.req, resp.err
}

func cliUploadFile(c *http.Client, uri, firstfn string) error {
	switch {
	case strings.HasSuffix(firstfn, ".deb"), strings.HasSuffix(firstfn, ".changes"):
		fns := []string{firstfn}
		for {
			if len(fns) == 0 {
				return nil
			}

			fn := fns[0]
			fns = fns[1:]

			log.Printf("Uploading %s\n", fn)

			req, err := newfileUploadRequest(uri, "debfiles", fn)
			if err != nil {
				return err
			}

			resp, err := c.Do(req)
			defer resp.Body.Close()
			if err != nil {
				return err
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			var status response
			err = json.Unmarshal(body, &status)
			if err != nil {
				return err
			}

			if status.Message.Complete {
				log.Printf("Completed %s", firstfn)
				return nil
			}

			for k, v := range status.Message.Expecting {
				if !v.Received {
					fns = append(fns, k)
				}
			}

			if len(fns) == 0 {
				return errors.New("Upload incomplete, but no files expected")
			}
		}
	default:
		return errors.New("Do not know how to upload ")
	}

	return nil
}

// CmdUpload is the implementation of the godinstall "upload" command
func CmdUpload(c *cli.Context) {
	ret := 0
	url := c.String("url")
	client := &http.Client{}

	for _, a := range c.Args() {
		err := cliUploadFile(client, url, a)

		if err != nil {
			log.Printf("Upload of %s failed, %s", a, err.Error())
			ret = 1
		}
	}

	os.Exit(ret)
}
