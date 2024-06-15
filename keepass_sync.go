package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	// "google.golang.org/api/drive/v2"
	"google.golang.org/api/option"

	"google.golang.org/api/drive/v3"

	"github.com/tobischo/gokeepasslib/v3"
)

// ####################
// # Keepass		  #
// ####################
type KeepasDBSync struct {
	localKeepassDB      *gokeepasslib.Database
	remoteKeepassDBCopy *gokeepasslib.Database
	syncKeepassDB       *gokeepasslib.Database
}

func newKeepasDBSync(localDBFileObj *os.File, remoteDBCopyFileObj *os.File, pass string) (*KeepasDBSync, error) {
	localDB := gokeepasslib.NewDatabase()
	remoteDBCopy := gokeepasslib.NewDatabase()

	cred := gokeepasslib.NewPasswordCredentials(pass)
	localDB.Credentials = cred
	remoteDBCopy.Credentials = cred

	syncDB := gokeepasslib.NewDatabase(
		gokeepasslib.WithDatabaseKDBXVersion4(),
	)

	err := gokeepasslib.NewDecoder(localDBFileObj).Decode(localDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize local Keepass DB: %v", err)
	}
	err = gokeepasslib.NewDecoder(remoteDBCopyFileObj).Decode(remoteDBCopy)
	if err != nil {
		return nil, fmt.Errorf("can't initialize remote Keepass DB copy: %v", err)
	}

	localDB.UnlockProtectedEntries()
	defer localDB.LockProtectedEntries()
	remoteDBCopy.UnlockProtectedEntries()
	defer remoteDBCopy.LockProtectedEntries()
	defer syncDB.LockProtectedEntries()

	// TODO: here is the potential bug!
	syncDB.Content = localDB.Content
	syncDB.Credentials = localDB.Credentials

	return &KeepasDBSync{
		localKeepassDB:      localDB,
		remoteKeepassDBCopy: remoteDBCopy,
		syncKeepassDB:       syncDB,
	}, nil
}

func (keepassSync KeepasDBSync) saveSyncwDB(fileName string) error {
	newFile, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("can't create new Keepass DB file: %v", err)
	}
	defer newFile.Close()

	// newKeepassDB.LockProtectedEntries()
	keepassEncoder := gokeepasslib.NewEncoder(newFile)
	if err := keepassEncoder.Encode(keepassSync.syncKeepassDB); err != nil {
		return fmt.Errorf("can't encode new keepass DB: %v", err)
	}

	return nil
}

func (keepasDBSync KeepasDBSync) syncBases() error {
	localEntries := keepasDBSync.localKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepasDBSync.remoteKeepassDBCopy.Content.Root.Groups[0].Entries

	var missingInLocal []gokeepasslib.Entry
	var missingInRemote []gokeepasslib.Entry
	var newEntries []gokeepasslib.Entry

	// convert entries lists to maps to ease search by uuid
	mapLocalEntries := make(map[string]gokeepasslib.Entry)
	mapRemoteEntries := make(map[string]gokeepasslib.Entry)
	for _, entry := range localEntries {
		mapLocalEntries[fmt.Sprintf("%x", entry.UUID)] = entry
	}
	for _, entry := range remoteCopyEntries {
		mapRemoteEntries[fmt.Sprintf("%x", entry.UUID)] = entry
	}

	// find missing entries in remote Keepass DB and for matching uuids find the latest modified version
	for localKey, localValue := range mapLocalEntries {
		remoteValue, ok := mapRemoteEntries[localKey]
		if !ok {
			missingInRemote = append(missingInRemote, localValue)
		} else {
			// TODO: LastModificationTime could be nil
			if remoteValue.Times.LastModificationTime.Time.After(localValue.Times.LastModificationTime.Time) {
				newEntries = append(newEntries, remoteValue)
			} else {
				newEntries = append(newEntries, localValue)
			}
		}
	}
	// find missing entries in local Keepass DB
	for remoteKey, remoteValue := range mapRemoteEntries {
		_, ok := mapLocalEntries[remoteKey]
		if !ok {
			missingInLocal = append(missingInLocal, remoteValue)
		}
	}
	// add missing entries
	newEntries = append(newEntries, missingInLocal...)
	newEntries = append(newEntries, missingInRemote...)
	keepasDBSync.syncKeepassDB.Content.Root.Groups[0].Entries = newEntries

	err := keepasDBSync.saveSyncwDB("tmp.kdbx")
	if err != nil {
		return fmt.Errorf("can't save new keepas DB: %v", err)
	}

	return nil
}

// ####################
// # Google Drive	  #
// ####################
type GoogleDriveController struct {
	service *drive.Service
}

func (controller *GoogleDriveController) listFiles(limit int64) (*drive.FileList, error) {
	r, err := controller.service.Files.List().PageSize(limit).
		Fields("nextPageToken, files(id, name)").Do()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (controller *GoogleDriveController) findFile(filename string) (*drive.File, error) {
	fileListResponse, err := controller.service.Files.List().Do()
	if err != nil {
		return nil, err
	}
	if len(fileListResponse.Files) == 0 {
		return nil, fmt.Errorf("google drive is empty")
	}

	for _, item := range fileListResponse.Files {
		if item.Name == filename {
			return item, nil
		}
	}

	return nil, fmt.Errorf("file not found on google drive")
}

func (controller GoogleDriveController) downloadRemoteKeepassDB(fileName string) error {
	remoteKeepassDb, err := controller.findFile(fileName)
	if err != nil {
		return fmt.Errorf("google drive error: %v", err)
	}
	googleDriveFileObj, err := controller.service.Files.Get(remoteKeepassDb.Id).Download()
	if err != nil {
		return fmt.Errorf("download error: %v", err)
	}

	localCopy, err := os.Create(fmt.Sprintf("remote_copy_%s", fileName))
	if err != nil {
		return fmt.Errorf("can't create local copy: %v", err)
	}
	defer localCopy.Close()

	_, err = io.Copy(localCopy, googleDriveFileObj.Body)
	if err != nil {
		return fmt.Errorf("can't copy remote db locally: %v", err)
	}

	return nil
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	// TODO: automate this parts
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func newGoogleDriveController(credentialsPath string) (*GoogleDriveController, error) {
	ctx := context.Background()
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	controller := GoogleDriveController{
		service: srv,
	}

	return &controller, nil
}

func initKeepasDBs(dbFileName string, googleDriveController *GoogleDriveController) (*KeepasDBSync, error) {
	localKeepassDBPath := fmt.Sprintf("/Users/semen-rudkovskiy/%s", dbFileName)

	localKeepassDBObj, err := os.Open(localKeepassDBPath)
	if err != nil {
		return nil, fmt.Errorf("can't open local Keepass DB file: %v", err)
	}
	defer localKeepassDBObj.Close()

	err = googleDriveController.downloadRemoteKeepassDB(dbFileName)
	if err != nil {
		return nil, fmt.Errorf("can't download remote Keepass DB file: %v", err)
	}
	// TODO: make the tmp file name prefix a setting
	remoteDBCopyObj, err := os.Open(fmt.Sprintf("remote_copy_%s", dbFileName))
	if err != nil {
		return nil, fmt.Errorf("can't open remote DB copy: %v", err)
	}
	defer remoteDBCopyObj.Close()

	// TODO: remove hard code
	keepasSync, err := newKeepasDBSync(localKeepassDBObj, remoteDBCopyObj, "1")
	if err != nil {
		log.Fatalf("can't open one of Keepass DBs: %v", err)
	}

	return keepasSync, nil
}

func main() {
	log.SetPrefix("### ")
	credentials := "client_credentials.json"
	// timeLayout := "2006-01-02T15:04:05.000Z"
	fileName := "test1.kdbx"

	googleDriveController, err := newGoogleDriveController(credentials)
	if err != nil {
		log.Fatalf("Unable initialize google drive: %v", err)
	}

	keepasSync, err := initKeepasDBs(fileName, googleDriveController)
	if err != nil {
		log.Fatalf("Unable to open one of Keepass DBs: %v", err)
	}

	keepasSync.syncBases()

	//############### print-debug part :)
	localEntries := keepasSync.localKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepasSync.remoteKeepassDBCopy.Content.Root.Groups[0].Entries
	fmt.Println("Local DB:")
	fmt.Printf("UUID: %x\n", localEntries[0].UUID)
	fmt.Printf("Time: %v\n", localEntries[0].Times.LastModificationTime)
	fmt.Println("Remote DB copy:")
	fmt.Printf("UUID: %x\n", remoteCopyEntries[0].UUID)
	fmt.Printf("Time: %v\n", remoteCopyEntries[0].Times.LastModificationTime)
}
