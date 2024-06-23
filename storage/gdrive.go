package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"kdbxsync/settings"
)

type googleDriveController struct {
	service    *drive.Service
	dbSettings *settings.DataBaseSettings
}

func (controller *googleDriveController) ListFiles(limit int64) (*drive.FileList, error) {
	r, err := controller.service.Files.List().PageSize(limit).
		Fields("nextPageToken, files(id, name, mimeType)").Do()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (controller *googleDriveController) Find(name string) (*drive.File, error) {
	query := fmt.Sprintf("name = '%s'and trashed = false", name)
	fileListResponse, err := controller.service.Files.List().Q(query).Fields("files(id, name, mimeType, parents)").Do()

	if err != nil {
		return nil, fmt.Errorf("file not found on google drive: %w", err)
	}
	if len(fileListResponse.Files) == 0 {
		return nil, errors.New("google drive is empty")
	}

	return fileListResponse.Files[0], nil
}

func (controller *googleDriveController) BackupDBFile() error {
	backupFolder, err := controller.Find("Backups")
	if err != nil {
		return fmt.Errorf("can't find backup folder: %w", err)
	}
	keepasDBFile, err := controller.Find(controller.dbSettings.FileName)

	if err != nil {
		return fmt.Errorf("can't find %s: %w", controller.dbSettings.FileName, err)
	}

	nowTimeStamp := time.Now()
	backupName := fmt.Sprintf("%s-%s", nowTimeStamp.Format("2006-01-02T15:04:05"), controller.dbSettings.FileName)
	backupFile := &drive.File{
		Name:    backupName,
		Parents: []string{backupFolder.Id},
	}

	_, err = controller.service.Files.Copy(keepasDBFile.Id, backupFile).Do()
	if err != nil {
		return fmt.Errorf("can't create backup: %w", err)
	}

	return nil
}

func (controller *googleDriveController) UpdateDBFile() error {
	filePath := controller.dbSettings.FullFilePath()

	fileObj, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("can't open db file: %w", err)
	}
	defer fileObj.Close()

	googleDriveDBFile, err := controller.Find(controller.dbSettings.FileName)
	if err != nil {
		return fmt.Errorf("can't find db file on google drive: %w", err)
	}

	fileMetaData := &drive.File{
		Name:     googleDriveDBFile.Name,
		MimeType: googleDriveDBFile.MimeType,
	}
	_, err = controller.service.Files.Update(googleDriveDBFile.Id, fileMetaData).Media(fileObj).Do()

	if err != nil {
		return fmt.Errorf("can't upload file on gogle drive: %w", err)
	}

	return nil
}

func (controller *googleDriveController) DownloadRemoteKeepassDB() error {
	remoteKeepassDB, err := controller.Find(controller.dbSettings.FileName)
	if err != nil {
		return fmt.Errorf("google drive error: %w", err)
	}
	googleDriveFileObj, err := controller.service.Files.Get(remoteKeepassDB.Id).Download()
	if err != nil {
		return fmt.Errorf("download error: %w", err)
	}

	localCopy, err := os.Create(controller.dbSettings.FullRemoteCopyFilePath())
	if err != nil {
		return fmt.Errorf("can't create local copy: %w", err)
	}
	defer localCopy.Close()

	_, err = io.Copy(localCopy, googleDriveFileObj.Body)
	if err != nil {
		return fmt.Errorf("can't copy remote db locally: %w", err)
	}

	err = googleDriveFileObj.Body.Close()
	if err != nil {
		return err
	}

	return nil
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, settings *settings.AppSettings) (*http.Client, error) {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		newTok, webTokenErr := getTokenFromWeb(config, settings)
		if webTokenErr != nil {
			return nil, webTokenErr
		}
		tokenErr := saveToken(tokFile, newTok)
		if tokenErr != nil {
			return nil, err
		}
		tok = newTok
	}
	return config.Client(context.Background(), tok), nil
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config, settings *settings.AppSettings) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// open authURL in browser *specific for macos
	command := exec.Command("open", authURL)
	err := command.Run()
	if err != nil {
		return nil, fmt.Errorf("exec error: %w", err)
	}
	// run goroutine to listen for callback
	go settings.HTTPServer.RunHTTPServer()
	// get the code from callback
	err = <-settings.HTTPServer.ErrorChannel
	if err != nil {
		return nil, fmt.Errorf("goroutine error: %w", err)
	}
	msg := <-settings.HTTPServer.ReturnChannel

	tok, err := config.Exchange(context.TODO(), msg)
	if err != nil {
		return nil, fmt.Errorf("can't retrieve token from web %w", err)
	}

	return tok, nil
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
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("can't cache oauth token: %w", err)
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(token)
	if err != nil {
		return err
	}

	return nil
}

func newGoogleDriveController(appSettings *settings.AppSettings) (*googleDriveController, error) {
	ctx := context.Background()
	b, err := os.ReadFile(appSettings.StorageCredentials)
	if err != nil {
		return nil, fmt.Errorf("can't read client secret file: %w", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("can't parse client secret file to config: %w", err)
	}
	client, err := getClient(config, appSettings)
	if err != nil {
		return nil, err
	}

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	controller := googleDriveController{
		service:    srv,
		dbSettings: appSettings.DatabaseSettings,
	}

	return &controller, nil
}
