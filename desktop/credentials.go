package main

type credentialStore interface {
	Save(deploymentID, apiKey string) error
	Load(deploymentID string) (string, error)
	Delete(deploymentID string) error
}

type windowsCredentialStore struct{}

func (windowsCredentialStore) Save(deploymentID, apiKey string) error {
	return saveDeploymentAPIKey(deploymentID, apiKey)
}

func (windowsCredentialStore) Load(deploymentID string) (string, error) {
	return loadDeploymentAPIKey(deploymentID)
}

func (windowsCredentialStore) Delete(deploymentID string) error {
	return deleteDeploymentAPIKey(deploymentID)
}
