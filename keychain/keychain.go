package keychain

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/keybase/go-keychain"

	"keepass_sync/http"
)

type KeychainAccess struct {
	Service     string `json:"service"`
	Account     string `json:"account"`
	Label       string `json:"label"`
	AccessGroup string `json:"access_group"`
}

func (keychainAccess *KeychainAccess) validateFields() error {
	if keychainAccess.Service == "" {
		return fmt.Errorf("service key is empty")
	}
	if keychainAccess.Account == "" {
		return fmt.Errorf("account key is empty")
	}
	if keychainAccess.Label == "" {
		return fmt.Errorf("label key is empty")
	}
	if keychainAccess.AccessGroup == "" {
		return fmt.Errorf("accessGroup key is empty")
	}
	return nil
}

func readKeychaiAccess(path string) (*KeychainAccess, error) {
	// f, err := os.Open(file)
	// if err != nil {
	// 	return nil, err
	// }
	// defer f.Close()
	// tok := &oauth2.Token{}
	// err = json.NewDecoder(f).Decode(tok)
	// return tok, err

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't read keychain access: %v", err)
	}
	kechainAccess := &KeychainAccess{}
	defer f.Close()
	// TODO: !!!!BUG when json is not valid no error arise
	err = json.NewDecoder(f).Decode(kechainAccess)
	if err != nil {
		return nil, err
	}
	err = kechainAccess.validateFields()
	if err != nil {
		return nil, fmt.Errorf("json error: %v", err)
	}

	return kechainAccess, nil
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

func (access KeychainAccess) GetPassword(callbackHttpServer *http.HttpServer) (string, error) {
	pass, err := access.getKeychainPass()
	if err != nil {
		return "", fmt.Errorf("can't get pass: %v", err)
	}
	if pass == "" {
		// TODO: remove hardcode
		url := "http://localhost:3030/missing_pass"
		// run goroutine to listen for callback
		go callbackHttpServer.RunHttpServer()
		// open localhost in browser to get pass from user *specific for macos
		command := exec.Command("open", url)
		// time.Sleep(10 * time.Second)
		commandErr := command.Run()
		if commandErr != nil {
			return "", fmt.Errorf("can't exec: %v", commandErr)
		}
		// get the code from callback
		pass = <-callbackHttpServer.Channel
		keychainError := access.newKeychainPass(pass)
		if keychainError != nil {
			return "", fmt.Errorf("can't add pass to keychain: %v", err)
		}
	}

	return pass, nil
}

func NewKeychainAccess(path string) (*KeychainAccess, error) {
	// keychainAccess := KeychainAccess{
	// 	Service:     "keepass_scripts",
	// 	account:     "keepass_sync",
	// 	label:       "keepass_sync",
	// 	accessGroup: "424242.group.com.example",
	// }
	keychainAccess, err := readKeychaiAccess(path)
	if err != nil {
		return nil, err
	}

	return keychainAccess, nil
}
