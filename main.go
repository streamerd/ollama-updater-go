package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type LocalModel struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type RemoteModelInfo map[string]interface{}

type ApiResponse struct {
	Models []LocalModel `json:"models"`
}

func main() {
	checkFlag := flag.Bool("check", false, "Check for outdated models")
	updateFlag := flag.Bool("update", false, "Update outdated models")

	flag.Parse()

	if *checkFlag || *updateFlag {
		// Fetch local models
		localEndpoint := "http://localhost:11434/api/tags"
		localResp, err := http.Get(localEndpoint)
		if err != nil {
			log.Fatalf("Failed to fetch local models: %v", err)
		}
		defer localResp.Body.Close()

		localBody, err := io.ReadAll(localResp.Body)
		if err != nil {
			log.Fatalf("Failed to read local models: %v", err)
		}

		var apiResponse ApiResponse
		err = json.Unmarshal(localBody, &apiResponse)
		if err != nil {
			log.Fatalf("Failed to parse local models: %v", err)
		}

		localModels := apiResponse.Models

		// Function to calculate hash of a JSON object
		calculateHash := func(jsonObj interface{}) string {
			jsonData, _ := json.Marshal(jsonObj)
			hash := sha256.Sum256(jsonData)
			return base64.StdEncoding.EncodeToString(hash[:])
		}

		// Array to hold non-up-to-date models
		var nonUpToDateModels []string

		// Iterate over local models and compare with remote models
		for _, localModel := range localModels {
			localDigest := localModel.Digest
			repo, tag := strings.Split(localModel.Name, ":")[0], strings.Split(localModel.Name, ":")[1]

			// Conditionally prepend "/library" to the repo name if it doesn't contain "/"
			if !strings.Contains(repo, "/") {
				repo = fmt.Sprintf("library/%s", repo)
			}

			// Construct URL for the remote model with the potentially modified repo name
			remoteURL := fmt.Sprintf("https://ollama.ai/v2/%s/manifests/%s", repo, tag)
			// Fetch remote model info
			remoteResp, err := http.Get(remoteURL)
			if err != nil {
				log.Printf("Failed to fetch remote model %s: %v\n", localModel.Name, err)
				continue
			}
			defer remoteResp.Body.Close()

			// Check for HTTP status codes indicating success (e.g., 200 OK)
			if remoteResp.StatusCode != http.StatusOK {
				log.Printf("Remote model %s not found or inaccessible.\n", localModel.Name)
				continue // Skip this model and continue with the next one
			}

			// Log the Content-Type header for debugging
			// contentType := remoteResp.Header.Get("Content-Type")
			// log.Printf("Content-Type for %s: %s\n", localModel.Name, contentType)

			// Read and log the raw response body for debugging
			remoteBody, err := io.ReadAll(remoteResp.Body)
			if err != nil {
				log.Printf("Failed to read remote model %s: %v\n", localModel.Name, err)
				continue
			}
			// log.Printf("Raw response body for %s: %s\n", localModel.Name, string(remoteBody))

			// Attempt to unmarshal the JSON
			var remoteModelInfo RemoteModelInfo
			err = json.Unmarshal(remoteBody, &remoteModelInfo)
			if err != nil {
				log.Printf("Failed to parse remote model %s: %v\n", localModel.Name, err)
				continue
			}

			// Calculate hash of the remote model info
			remoteHash := calculateHash(remoteModelInfo)

			// Compare hashes
			if remoteHash == localDigest {
				log.Printf("You have the latest %s\n", localModel.Name)
			} else {
				// log.Printf("You have an outdated version of %s\n", localModel.Name)
				nonUpToDateModels = append(nonUpToDateModels, localModel.Name)

				if *updateFlag {
					log.Printf("Updating %s\n", localModel.Name)
					updateModel(localModel.Name)
				}
			}
		}

		if *checkFlag {
			if len(nonUpToDateModels) > 0 {
				log.Println("Non-up-to-date models:")
				for _, modelName := range nonUpToDateModels {
					log.Println("-", modelName)
				}
			} else {
				log.Println("All models are up to date.")
			}
		}
	} else {
		log.Fatal("Please specify either -check or -update flag.")
	}
}

// Function to update a model
func updateModel(name string) {
	pullURL := "http://localhost:11434/api/pull"
	payload := map[string]string{"name": name}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Error marshaling payload: %v", err)
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", pullURL, body)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Handle streamed response
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			log.Fatalf("Error reading response body: %v", err)
		}
		if n == 0 {
			break
		}

		// Process the chunk of data here
		fmt.Print(string(buf[:n]))
	}
}
