package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type LocalModel struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type RemoteModelInfo map[string]interface{}

type ApiResponse struct {
	Models []LocalModel `json:"models"`
}

var focusedIndex int
var selectedIndices []int
var focusedStyle tcell.Style

func init() {
	// Initialize the focused style with a distinctive background color
	focusedStyle = tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack)
}

func main() {
	app := tview.NewApplication()

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

		// Read the raw response body
		remoteBody, err := io.ReadAll(remoteResp.Body)
		if err != nil {
			log.Printf("Failed to read remote model %s: %v\n", localModel.Name, err)
			continue
		}

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
		if remoteHash != localDigest {
			nonUpToDateModels = append(nonUpToDateModels, localModel.Name)
		}
	}

	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	all := tview.NewCheckbox()
	all.SetLabel("All")
	all.SetBackgroundColor(tcell.Color102)
	flex.AddItem(all, 1, 1, true)

	checkboxes := []*tview.Checkbox{}
	for _, model := range localModels {
		cb := tview.NewCheckbox()
		cb.SetLabel(model.Name)
		checkboxes = append(checkboxes, cb)
	}

	for _, cb := range checkboxes {
		flex.AddItem(cb, 1, 1, false)
	}

	// Attach the change handler to each checkbox
	for i, cb := range checkboxes {
		cb.SetChangedFunc(func(checked bool) {
			handleCheckboxChange(checkboxes, i, checked, cb.GetLabel(), app)
		})
	}

	// Define a function to handle input events
	handleInput := func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() { // Use event.Key() instead of event.Rune()
		case tcell.KeyEnter: // Use tcell.KeyEnter for the Enter key
			// Your existing logic for handling Enter key
		case tcell.KeyUp: // Correctly match tcell.KeyUp
			if focusedIndex > 0 {
				focusedIndex--
				updateFocusVisual(checkboxes, app)
			}
		case tcell.KeyDown: // Correctly match tcell.KeyDown
			if focusedIndex < len(checkboxes)-1 {
				focusedIndex++
				updateFocusVisual(checkboxes, app)
			}
		}

		return event
	}

	// Attach the input handler to the flex container
	flex.SetInputCapture(handleInput)

	// Run the application with the flex container as root
	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}

}

func handleCheckboxChange(checkboxes []*tview.Checkbox, index int, checked bool, itemText string, app *tview.Application) {
	if itemText == "All" {
		for i := 0; i < len(checkboxes); i++ {
			checkboxes[i].SetChecked(checked)
			if checked {
				checkboxes[i].SetDisabled(true)
			} else {
				checkboxes[i].SetDisabled(false)
			}
		}
	} else {
		allChecked := true
		for _, cb := range checkboxes {
			if cb.IsChecked() {
				allChecked = false
				break
			}
		}
		checkboxes[0].SetChecked(allChecked)
		if allChecked {
			checkboxes[0].SetDisabled(true)
		} else {
			checkboxes[0].SetDisabled(false)
		}
	}
}

func updateFocusVisual(checkboxes []*tview.Checkbox, app *tview.Application) {
	// Update the visual appearance of the focused checkbox
	for i, cb := range checkboxes {
		if i == focusedIndex {
			// Apply the focusedStyle with a modified background color directly
			cb.SetBackgroundColor(tcell.Color100) // Apply the modified style directly
		} else {
			// Reset the background color to the default or another color as needed
			cb.SetBackgroundColor(tcell.ColorDefault) // Example: Reset to default color
		}
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
