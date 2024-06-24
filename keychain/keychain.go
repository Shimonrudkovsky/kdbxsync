package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/keybase/go-keychain"

	"kdbxsync/http"
)

type KeyStorage interface {
	GetPassword(http.HTTPServer) (string, error)
}

type Access struct {
	Service     string `json:"service"`
	Account     string `json:"account"`
	Label       string `json:"label"`
	AccessGroup string `json:"access_group"`
}

func (keychainAccess *Access) validateFields() error {
	if keychainAccess.Service == "" {
		return errors.New("service key is empty")
	}
	if keychainAccess.Account == "" {
		return errors.New("account key is empty")
	}
	if keychainAccess.Label == "" {
		return errors.New("label key is empty")
	}
	if keychainAccess.AccessGroup == "" {
		return errors.New("accessGroup key is empty")
	}
	return nil
}

func (keychainAccess Access) newKeychainPass(pass string) error {
	newPass := keychain.NewGenericPassword(
		keychainAccess.Service,
		keychainAccess.Account,
		keychainAccess.Label, []byte(pass),
		keychainAccess.AccessGroup,
	)
	newPass.SetSynchronizable(keychain.SynchronizableNo)
	newPass.SetAccessible(keychain.AccessibleAccessibleAlwaysThisDeviceOnly)

	err := keychain.AddItem(newPass)
	if err != nil {
		return fmt.Errorf("can't add password to the keychain: %w", err)
	}

	return nil
}

func (keychainAccess Access) getKeychainPass() (string, error) {
	resp, err := keychain.GetGenericPassword(
		keychainAccess.Service,
		keychainAccess.Account,
		keychainAccess.Label, keychainAccess.AccessGroup,
	)
	if err != nil {
		return "", fmt.Errorf("can't get password from keychain: %w", err)
	} else if len(resp) < 1 {
		return "", nil
	}
	return string(resp[:]), nil
}

func (keychainAccess Access) GetPassword(callbackHTTPServer http.HTTPServer) (string, error) {
	pass, err := keychainAccess.getKeychainPass()
	if err != nil {
		return "", fmt.Errorf("can't get pass: %w", err)
	}
	if pass == "" {
		// TODO: remove hardcode
		url := "http://localhost:3030/missing_pass"
		// run goroutine to listen for callback
		go callbackHTTPServer.RunHTTPServer()
		// open localhost in browser to get pass from user *specific for macos
		command := exec.Command("open", url)
		commandErr := command.Run()
		if commandErr != nil {
			return "", fmt.Errorf("can't exec: %w", commandErr)
		}
		// get the code from callback
		pass, err = callbackHTTPServer.ReadChannels()
		if err != nil {
			return "", err
		}
		keychainError := keychainAccess.newKeychainPass(pass)
		if keychainError != nil {
			return "", fmt.Errorf("can't add pass to keychain: %w", err)
		}
	}

	return pass, nil
}

func readKeychaiAccess(path string) (*Access, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't read keychain access: %w", err)
	}
	defer f.Close()
	kechainAccess := &Access{}
	err = json.NewDecoder(f).Decode(kechainAccess)
	if err != nil {
		return nil, err
	}
	err = kechainAccess.validateFields()
	if err != nil {
		return nil, fmt.Errorf("json error: %w", err)
	}

	return kechainAccess, nil
}

func NewKeychainAccess(path string) (*Access, error) {
	keychainAccess, err := readKeychaiAccess(path)
	if err != nil {
		return nil, err
	}

	return keychainAccess, nil
}
