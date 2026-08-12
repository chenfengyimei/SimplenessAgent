//go:build !windows

package main

import "fmt"

func saveDeploymentAPIKey(_ string, _ string) error {
	return fmt.Errorf("secure credential storage is only implemented for Windows desktop builds")
}

func loadDeploymentAPIKey(_ string) (string, error) {
	return "", fmt.Errorf("secure credential storage is only implemented for Windows desktop builds")
}

func deleteDeploymentAPIKey(_ string) error {
	return fmt.Errorf("secure credential storage is only implemented for Windows desktop builds")
}
