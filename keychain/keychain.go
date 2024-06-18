package keychain

import (
	"fmt"
	"os/exec"

	"github.com/keybase/go-keychain"

	"keepass_sync/http_server"
)

type KeychainAccess struct {
	Service     string
	Account     string
	Label       string
	AccessGroup string
}

func (access KeychainAccess) newKeychainPass(pass string) error {
	new_pass := keychain.NewGenericPassword(access.Service, access.Account, access.Label, []byte(pass), access.AccessGroup)
	new_pass.SetSynchronizable(keychain.SynchronizableNo)
	new_pass.SetAccessible(keychain.AccessibleAccessibleAlwaysThisDeviceOnly)

	err := keychain.AddItem(new_pass)
	if err != nil {
		return fmt.Errorf("can't add password to the keychain: %v", err)
	}

	return nil
}

func (access KeychainAccess) getKeychainPass() (string, error) {
	resp, err := keychain.GetGenericPassword(access.Service, access.Account, access.Label, access.AccessGroup)
	if err != nil {
		return "", fmt.Errorf("can't get password from keychain: %v", err)
	} else if len(resp) < 1 {
		return "", nil
	} else {
		return string(resp[:]), nil
	}
}

func (access KeychainAccess) GetPassword() (string, error) {
	channel := make(chan string)
	tmpHttpServer := http_server.HttpServer{Channel: channel}
	pass, err := access.getKeychainPass()
	if err != nil {
		return "", fmt.Errorf("can't get pass: %v", err)
	}
	if pass == "" {
		url := "http://localhost:3030/missing_pass"
		// run goroutine to listen for callback
		go tmpHttpServer.RunHttpServer()
		// open localhost in browser to get pass from user *specific for macos
		command := exec.Command("open", url)
		// time.Sleep(10 * time.Second)
		commandErr := command.Run()
		if commandErr != nil {
			return "", fmt.Errorf("can't exec: %v", commandErr)
		}
		// get the code from callback
		pass = <-channel
		keychainError := access.newKeychainPass(pass)
		if keychainError != nil {
			return "", fmt.Errorf("can't add pass to keychain: %v", err)
		}
	}

	return pass, nil
}
