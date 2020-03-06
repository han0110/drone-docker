package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

func main() {
	var (
		repo     = getenv("PLUGIN_REPO")
		registry = getenv("PLUGIN_REGISTRY")
		password = getenv(
			"PLUGIN_JSON_KEY",
			"GCR_JSON_KEY",
			"GOOGLE_CREDENTIALS",
			"TOKEN",
		)
		// default username
		username = "_json_key"
	)

	// decode the token if base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(password)
	if err == nil {
		password = string(decoded)
	}

	// default registry value
	if registry == "" {
		registry = "gcr.io"
	}

	// must use the fully qualified repo name. If the
	// repo name does not have the registry prefix we
	// should prepend.
	if !strings.HasPrefix(repo, registry) {
		repo = path.Join(registry, repo)
	}

	// fallback to use access token when password is not found
	if password == "" {
		accessToken, err := getGCEAccessToken()
		if err != nil {
			log.Fatal(err)
		}
		username = "oauth2accesstoken"
		password = accessToken
	}

	os.Setenv("PLUGIN_REPO", repo)
	os.Setenv("PLUGIN_REGISTRY", registry)
	os.Setenv("DOCKER_USERNAME", username)
	os.Setenv("DOCKER_PASSWORD", password)

	// invoke the base docker plugin binary
	cmd := exec.Command("drone-docker")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		os.Exit(1)
	}
}

func getenv(key ...string) (s string) {
	for _, k := range key {
		s = os.Getenv(k)
		if s != "" {
			return
		}
	}
	return
}

func getGCEAccessToken() (string, error) {
	client := newMetadataClient()
	saEmail, err := client.GetServiceAccountEmail()
	if err != nil {
		return "", err
	}
	accessToken, err := client.GetAccessToken(saEmail)
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

type metadataClient struct {
	httpclient *http.Client
	endpoint   string
}

func newMetadataClient() *metadataClient {
	return &metadataClient{
		httpclient: &http.Client{
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
			},
		},
		endpoint: "http://metadata.google.internal/computeMetadata/v1",
	}
}

func (client *metadataClient) GetAccessToken(saEmail string) (string, error) {
	var data struct {
		AccessToken string `json:"access_token"`
	}
	path := fmt.Sprintf("/instance/service-accounts/%s/token", saEmail)
	if err := client.get(&data, path); err != nil {
		return "", err
	}
	return data.AccessToken, nil
}

func (client *metadataClient) GetServiceAccountEmail() (string, error) {
	var data struct {
		Default struct{ Email string }
	}
	path := "/instance/service-accounts/?recursive=true"
	if err := client.get(&data, path); err != nil {
		return "", err
	}
	return data.Default.Email, nil
}

func (client *metadataClient) get(target interface{}, path string) error {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s%s", client.endpoint, path), nil)
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.httpclient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}
