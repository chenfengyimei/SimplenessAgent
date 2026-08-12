//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric   = 1
	credentialPersistLocal  = 2
	credentialManagerPrefix = "SimplenessAgent/deployment/"
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func saveDeploymentAPIKey(deploymentID, apiKey string) error {
	target, err := windows.UTF16PtrFromString(credentialManagerPrefix + deploymentID)
	if err != nil {
		return err
	}
	blob := []byte(apiKey)
	credential := windowsCredential{Type: credentialTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(blob)), Persist: credentialPersistLocal}
	if len(blob) > 0 {
		credential.CredentialBlob = &blob[0]
	}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if result == 0 {
		return fmt.Errorf("Windows Credential Manager write: %w", callErr)
	}
	return nil
}

func loadDeploymentAPIKey(deploymentID string) (string, error) {
	target, err := windows.UTF16PtrFromString(credentialManagerPrefix + deploymentID)
	if err != nil {
		return "", err
	}
	var credential *windowsCredential
	result, _, callErr := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0, uintptr(unsafe.Pointer(&credential)))
	if result == 0 {
		return "", fmt.Errorf("Windows Credential Manager read: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential == nil || credential.CredentialBlobSize == 0 || credential.CredentialBlob == nil {
		return "", fmt.Errorf("Windows Credential Manager returned an empty API Key")
	}
	return string(unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))), nil
}

func deleteDeploymentAPIKey(deploymentID string) error {
	target, err := windows.UTF16PtrFromString(credentialManagerPrefix + deploymentID)
	if err != nil {
		return err
	}
	result, _, callErr := procCredDeleteW.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0)
	if result == 0 {
		return fmt.Errorf("Windows Credential Manager delete: %w", callErr)
	}
	return nil
}
