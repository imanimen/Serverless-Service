package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
)

type Function struct {
    Name     string            `json:"name"`
    Code     string            `json:"code"`
    Language string            `json:"language"`
    Env      map[string]string `json:"env"`
}

var redisClient *redis.Client

func main() {
	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")
	if redisHost == "" {
		redisHost = "localhost"
	}
	if redisPort == "" {
		redisPort = "6379"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
	})

	r := mux.NewRouter()
	r.HandleFunc("/function", createFunction).Methods("POST")
	r.HandleFunc("/function/{name}", executeFunction).Methods("POST")

	log.Println("server is listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func createFunction(w http.ResponseWriter, r *http.Request) {
	var function Function
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &function)

	err := redisClient.HSet(r.Context(), "functions", function.Name, body).Err()
	if err != nil {
		log.Printf("Error storing function in Redis: %v", err)
		http.Error(w, "Error storing function", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "function registered"})
}

func executeFunction(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    functionName := vars["name"]

    log.Printf("Attempting to retrieve function: %s", functionName)
    functionData, err := redisClient.HGet(r.Context(), "functions", functionName).Result()
    if err != nil {
        log.Printf("Error retrieving function from Redis: %v", err)
        http.Error(w, "function not found", http.StatusNotFound)
        return
    }
    log.Printf("Retrieved function data: %s", functionData)

    var function Function
    json.Unmarshal([]byte(functionData), &function)

    tmpDir, err := os.MkdirTemp("", "function-*")
    if err != nil {
        log.Printf("Error creating temporary directory: %v", err)
        http.Error(w, "Error executing function", http.StatusInternalServerError)
        return
    }
    defer os.RemoveAll(tmpDir)

    tempFile := filepath.Join(tmpDir, "main.go")
    if err := os.WriteFile(tempFile, []byte(function.Code), 0644); err != nil {
        log.Printf("Error writing temporary file: %v", err)
        http.Error(w, "Error executing function", http.StatusInternalServerError)
        return
    }

    cmd := exec.Command("go", "run", tempFile)
    cmd.Stdin = strings.NewReader(function.Code)
    output, err := cmd.CombinedOutput()
    if err != nil {
        log.Printf("Error executing function: %v", err)
        log.Printf("Output: %s", output)
        http.Error(w, fmt.Sprintf("Error executing function: %v\nOutput: %s", err, output), http.StatusInternalServerError)
        return
    }

    w.Write(output)
}