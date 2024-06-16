package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/option"

	"google.golang.org/api/drive/v3"

	"github.com/tobischo/gokeepasslib/v3"
)

type DataBaseSettings struct {
	directory        string
	fileName         string
	password         string
	remoteCopyPrefix string
	syncDBName       string
}

func backupKeepassDB(dbSettings DataBaseSettings) error {
	nowTimeStamp := time.Now()
	dbFilePath := fmt.Sprintf("%s/%s", dbSettings.directory, dbSettings.fileName)
	backupDirectory := fmt.Sprintf("%s/backups", dbSettings.directory)

	info, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't get local Keepass BD file info: %v", err)
	}
	data, err := os.ReadFile(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't read local Keepass DB file: %v", err)
	}

	// TODO: remove hardcode from permissions
	err = os.MkdirAll(backupDirectory, 0777)
	if err != nil {
		return fmt.Errorf("can't create backup directory: %v", err)
	}
	err = os.WriteFile(fmt.Sprintf("%s/%s-%s", backupDirectory, nowTimeStamp.Format("2006-01-02T15-04-05"), dbSettings.fileName), data, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("can't write a backup file: %v", err)
	}

	return nil
}

// ####################
// # Keepass		  #
// ####################
type KeepasDBSync struct {
	localKeepassDB      *gokeepasslib.Database
	remoteKeepassDBCopy *gokeepasslib.Database
	syncKeepassDB       *gokeepasslib.Database
}

func newKeepasDBSync(localDBFileObj *os.File, remoteDBCopyFileObj *os.File, pass string) (*KeepasDBSync, error) {
	// new db instances to decode files into
	localDB := gokeepasslib.NewDatabase()
	remoteDBCopy := gokeepasslib.NewDatabase()
	syncDB := gokeepasslib.NewDatabase()

	// pass
	cred := gokeepasslib.NewPasswordCredentials(pass)
	localDB.Credentials = cred
	remoteDBCopy.Credentials = cred
	syncDB.Credentials = cred

	// decoding local and remote copy
	err := gokeepasslib.NewDecoder(localDBFileObj).Decode(localDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize local Keepass DB: %v", err)
	}
	err = gokeepasslib.NewDecoder(remoteDBCopyFileObj).Decode(remoteDBCopy)
	if err != nil {
		return nil, fmt.Errorf("can't initialize remote Keepass DB copy: %v", err)
	}
	// copying content from local db into sync db to recreate content structure,
	// assuming that all entries would be rewritten after the sync
	// TODO: here is the potential bug!
	syncDB.Content = localDB.Content

	localDB.UnlockProtectedEntries()
	defer localDB.LockProtectedEntries()
	remoteDBCopy.UnlockProtectedEntries()
	defer remoteDBCopy.LockProtectedEntries()
	defer syncDB.LockProtectedEntries()

	return &KeepasDBSync{
		localKeepassDB:      localDB,
		remoteKeepassDBCopy: remoteDBCopy,
		syncKeepassDB:       syncDB,
	}, nil
}

func (keepassSync KeepasDBSync) saveSyncwDB(filePath string) error {
	newFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("can't create new Keepass DB file: %v", err)
	}
	defer newFile.Close()

	keepassEncoder := gokeepasslib.NewEncoder(newFile)
	if err := keepassEncoder.Encode(keepassSync.syncKeepassDB); err != nil {
		return fmt.Errorf("can't encode new keepass DB: %v", err)
	}

	return nil
}

func (keepasDBSync KeepasDBSync) syncBases(dbSettings DataBaseSettings) error {
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

	err := keepasDBSync.saveSyncwDB(fmt.Sprintf("%s/%s", dbSettings.directory, dbSettings.syncDBName))
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
		Fields("nextPageToken, files(id, name, mimeType)").Do()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (controller *GoogleDriveController) find(name string) (*drive.File, error) {
	query := fmt.Sprintf("name = '%s'and trashed = false", name)
	fileListResponse, err := controller.service.Files.List().Q(query).Fields("files(id, name, mimeType, parents)").Do()

	if err != nil {
		return nil, fmt.Errorf("file not found on google drive: %v", err)
	}
	if len(fileListResponse.Files) == 0 {
		return nil, fmt.Errorf("google drive is empty")
	}

	return fileListResponse.Files[0], nil
}

func (controller *GoogleDriveController) backupDBFile(dbSettings DataBaseSettings) error {
	backupFolder, err := controller.find("Backups")
	if err != nil {
		return fmt.Errorf("can't find backup folder: %v", err)
	}
	keepasDBFile, err := controller.find(dbSettings.fileName)
	if err != nil {
		return fmt.Errorf("can't find %s: %v", dbSettings.fileName, err)
	}

	nowTimeStamp := time.Now()
	backupName := fmt.Sprintf("%s-%s", nowTimeStamp.Format("2006-01-02T15:04:05"), dbSettings.fileName)
	backupFile := &drive.File{
		Name:    backupName,
		Parents: []string{backupFolder.Id},
	}

	_, err = controller.service.Files.Copy(keepasDBFile.Id, backupFile).Do()
	if err != nil {
		return fmt.Errorf("can't create backup: %v", err)
	}

	return nil
}

func (controller *GoogleDriveController) updateDBFile(dbSettings DataBaseSettings) error {
	filePath := fmt.Sprintf("%s/%s", dbSettings.directory, dbSettings.fileName)

	fileObj, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("can't open db file: %v", err)
	}
	defer fileObj.Close()

	googleDriveDBFile, err := controller.find(dbSettings.fileName)
	if err != nil {
		return fmt.Errorf("can't find db file on google drive: %v", err)
	}

	fileMetaData := &drive.File{
		Name:     googleDriveDBFile.Name,
		MimeType: googleDriveDBFile.MimeType,
	}
	_, err = controller.service.Files.Update(googleDriveDBFile.Id, fileMetaData).Media(fileObj).Do()

	if err != nil {
		return fmt.Errorf("can't upload file to gogle drive: %v", err)
	}

	return nil
}

func (controller GoogleDriveController) downloadRemoteKeepassDB(dbSettings DataBaseSettings) error {
	remoteKeepassDb, err := controller.find(dbSettings.fileName)
	if err != nil {
		return fmt.Errorf("google drive error: %v", err)
	}
	googleDriveFileObj, err := controller.service.Files.Get(remoteKeepassDb.Id).Download()
	if err != nil {
		return fmt.Errorf("download error: %v", err)
	}

	localCopy, err := os.Create(fmt.Sprintf("%s/%s_%s", dbSettings.directory, dbSettings.remoteCopyPrefix, dbSettings.fileName))
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

func initKeepasDBs(dbSettings DataBaseSettings, googleDriveController *GoogleDriveController) (*KeepasDBSync, error) {
	localKeepassDBPath := fmt.Sprintf("%s/%s", dbSettings.directory, dbSettings.fileName)

	localKeepassDBObj, err := os.Open(localKeepassDBPath)
	if err != nil {
		return nil, fmt.Errorf("can't open local Keepass DB file: %v", err)
	}
	defer localKeepassDBObj.Close()

	err = googleDriveController.downloadRemoteKeepassDB(dbSettings)
	if err != nil {
		return nil, fmt.Errorf("can't download remote Keepass DB file: %v", err)
	}

	remoteDBCopyObj, err := os.Open(fmt.Sprintf("%s/%s_%s", dbSettings.directory, dbSettings.remoteCopyPrefix, dbSettings.fileName))
	if err != nil {
		return nil, fmt.Errorf("can't open remote DB copy: %v", err)
	}
	defer remoteDBCopyObj.Close()

	keepasSync, err := newKeepasDBSync(localKeepassDBObj, remoteDBCopyObj, dbSettings.password)
	if err != nil {
		log.Fatalf("can't open one of Keepass DBs: %v", err)
	}

	return keepasSync, nil
}

func main() {
	dbSettings := DataBaseSettings{
		directory:        "/Users/semen-rudkovskiy/Documents/Keepass",
		fileName:         "test1.kdbx",
		password:         "1",
		remoteCopyPrefix: "remote_copy",
		syncDBName:       "tmp.kdbx",
	}
	log.SetPrefix("### ")
	credentials := "client_credentials.json"
	// timeLayout := "2006-01-02T15:04:05.000Z"

	googleDriveController, err := newGoogleDriveController(credentials)
	if err != nil {
		log.Fatalf("Unable initialize google drive: %v", err)
	}

	// backup first everything else later :)
	err = backupKeepassDB(dbSettings)
	if err != nil {
		log.Fatalf("Unable to create backup: %v", err)
	}

	err = googleDriveController.backupDBFile(dbSettings)
	if err != nil {
		log.Fatalf("Unable to backup remote base: %v", err)
	}

	keepasSync, err := initKeepasDBs(dbSettings, googleDriveController)
	if err != nil {
		log.Fatalf("Unable to open one of Keepass DBs: %v", err)
	}

	err = keepasSync.syncBases(dbSettings)
	if err != nil {
		log.Fatalf("Unable to sync bases: %v", err)
	}

	err = googleDriveController.updateDBFile(dbSettings)
	if err != nil {
		log.Fatalf("Unable to upload Keepass DB file to google drive: %v", err)
	}

	//############### print-debug part :)
	l, _ := googleDriveController.find("Backups")
	s, _ := json.MarshalIndent(l, "", "    ")
	fmt.Println(string(s))
	localEntries := keepasSync.localKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepasSync.remoteKeepassDBCopy.Content.Root.Groups[0].Entries
	fmt.Println("Local DB:")
	fmt.Printf("UUID: %x\n", localEntries[0].UUID)
	fmt.Printf("Time: %v\n", localEntries[0].Times.LastModificationTime)
	fmt.Println("Remote DB copy:")
	fmt.Printf("UUID: %x\n", remoteCopyEntries[0].UUID)
	fmt.Printf("Time: %v\n", remoteCopyEntries[0].Times.LastModificationTime)
}
