package google_drive

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

	"keepass_sync/settings"
)

type GoogleDriveController struct {
	service *drive.Service
}

func (controller *GoogleDriveController) ListFiles(limit int64) (*drive.FileList, error) {
	r, err := controller.service.Files.List().PageSize(limit).
		Fields("nextPageToken, files(id, name, mimeType)").Do()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (controller *GoogleDriveController) Find(name string) (*drive.File, error) {
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

func (controller *GoogleDriveController) BackupDBFile(dbSettings *settings.DataBaseSettings) error {
	backupFolder, err := controller.Find("Backups")
	if err != nil {
		return fmt.Errorf("can't find backup folder: %v", err)
	}
	keepasDBFile, err := controller.Find(dbSettings.FileName)
	if err != nil {
		return fmt.Errorf("can't find %s: %v", dbSettings.FileName, err)
	}

	nowTimeStamp := time.Now()
	backupName := fmt.Sprintf("%s-%s", nowTimeStamp.Format("2006-01-02T15:04:05"), dbSettings.FileName)
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

func (controller *GoogleDriveController) UpdateDBFile(dbSettings *settings.DataBaseSettings) error {
	filePath := dbSettings.FullFilePath()

	fileObj, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("can't open db file: %v", err)
	}
	defer fileObj.Close()

	googleDriveDBFile, err := controller.Find(dbSettings.FileName)
	if err != nil {
		return fmt.Errorf("can't find db file on google drive: %v", err)
	}

	fileMetaData := &drive.File{
		Name:     googleDriveDBFile.Name,
		MimeType: googleDriveDBFile.MimeType,
	}
	_, err = controller.service.Files.Update(googleDriveDBFile.Id, fileMetaData).Media(fileObj).Do()

	if err != nil {
		return fmt.Errorf("can't upload file on gogle drive: %v", err)
	}

	return nil
}

func (controller GoogleDriveController) DownloadRemoteKeepassDB(dbSettings *settings.DataBaseSettings) error {
	remoteKeepassDb, err := controller.Find(dbSettings.FileName)
	if err != nil {
		return fmt.Errorf("google drive error: %v", err)
	}
	googleDriveFileObj, err := controller.service.Files.Get(remoteKeepassDb.Id).Download()
	if err != nil {
		return fmt.Errorf("download error: %v", err)
	}

	localCopy, err := os.Create(dbSettings.FullRemoteCopyFilePath())
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

func NewGoogleDriveController(credentialsPath string) (*GoogleDriveController, error) {
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
