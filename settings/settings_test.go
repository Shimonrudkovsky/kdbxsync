package settings_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"kdbxsync/http"
	"kdbxsync/settings"

	"github.com/stretchr/testify/assert"
)

type FakeKeychainAccess struct {
	password string
	err      error
}

func (f *FakeKeychainAccess) GetPassword(server http.HTTPServer) (string, error) {
	return f.password, f.err
}

type FakeHTTPServer struct{}

func (fhs *FakeHTTPServer) RunHTTPServer() {}
func (fhs *FakeHTTPServer) ReadChannels() (string, error) {
	return "pass", nil
}

func TestGetEnvs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		testDirectoryEnvName := "KEEPASS_DB_DIRECTORY_TEST"
		testDirectoryEnvVal := "/test/directory"
		testDBFileNameEnvName := "KEEPASS_DB_FILE_NAME_TEST"
		testDBFileNameEnvVal := "testfile.kdbx"
		os.Setenv(testDirectoryEnvName, testDirectoryEnvVal)
		os.Setenv(testDBFileNameEnvName, testDBFileNameEnvVal)
		defer os.Unsetenv(testDirectoryEnvName)
		defer os.Unsetenv(testDBFileNameEnvName)

		vars, err := settings.GetEnvs(testDirectoryEnvName, testDBFileNameEnvName)

		assert.NoError(t, err)
		assert.NotNil(t, vars)
		assert.Equal(t, testDirectoryEnvVal, vars.Directory)
		assert.Equal(t, testDBFileNameEnvVal, vars.DBFileName)
	})
	t.Run("error: no directory variable", func(t *testing.T) {
		testDirectoryEnvName := ""
		testDirectoryEnvVal := "/test/directory"
		testDBFileNameEnvName := "KEEPASS_DB_FILE_NAME_TEST"
		testDBFileNameEnvVal := "testfile.kdbx"
		os.Setenv(testDirectoryEnvName, testDirectoryEnvVal)
		os.Setenv(testDBFileNameEnvName, testDBFileNameEnvVal)
		defer os.Unsetenv(testDirectoryEnvName)
		defer os.Unsetenv(testDBFileNameEnvName)

		vars, err := settings.GetEnvs(testDirectoryEnvName, testDBFileNameEnvName)

		assert.Error(t, err)
		assert.Nil(t, vars)
		assert.Equal(t, "can't find directory variable", err.Error())
	})
	t.Run("error: no db file name variable", func(t *testing.T) {
		testDirectoryEnvName := "KEEPASS_DB_DIRECTORY_TEST"
		testDirectoryEnvVal := "/test/directory"
		testDBFileNameEnvName := ""
		testDBFileNameEnvVal := "testfile.kdbx"
		os.Setenv(testDirectoryEnvName, testDirectoryEnvVal)
		os.Setenv(testDBFileNameEnvName, testDBFileNameEnvVal)
		defer os.Unsetenv(testDirectoryEnvName)
		defer os.Unsetenv(testDBFileNameEnvName)

		vars, err := settings.GetEnvs(testDirectoryEnvName, testDBFileNameEnvName)

		assert.Error(t, err)
		assert.Nil(t, vars)
		assert.Equal(t, "can't find db file name variable", err.Error())
	})
}

func TestDataBaseSettingsMethods(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		directory := "dir/path"
		fileNmae := "test.kdbx"
		pass := "pass"
		remoteCopyPrefix := "remote_copy"
		syncDBName := "tmp.kdbx"
		backupDir := "backup"
		dbSettings := settings.DataBaseSettings{
			Directory:        directory,
			FileName:         fileNmae,
			Password:         pass,
			RemoteCopyPrefix: remoteCopyPrefix,
			SyncDBName:       syncDBName,
			BackupDirectory:  backupDir,
		}

		fullFilePath := dbSettings.FullFilePath()
		fullRemoteCopyFilePath := dbSettings.FullRemoteCopyFilePath()
		fullSyncFilePath := dbSettings.FullSyncFilePath()

		assert.Equal(t, fullFilePath, fmt.Sprintf("%s/%s", dbSettings.Directory, dbSettings.FileName))
		assert.Equal(
			t,
			fullRemoteCopyFilePath,
			fmt.Sprintf("%s/%s_%s", dbSettings.Directory, dbSettings.RemoteCopyPrefix, dbSettings.FileName),
		)
		assert.Equal(t, fullSyncFilePath, fmt.Sprintf("%s/%s", dbSettings.Directory, dbSettings.SyncDBName))

	})
}

func TestNewDatabaseSetting(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		os.Setenv("KEEPASS_DB_DIRECTORY", "/test/directory")
		os.Setenv("KEEPASS_DB_FILE_NAME", "testfile.kdbx")
		defer os.Unsetenv("KEEPASS_DB_DIRECTORY")
		defer os.Unsetenv("KEEPASS_DB_FILE_NAME")

		fakeKeychainAccess := &FakeKeychainAccess{password: "testpassword", err: nil}
		fakeHTTPServer := &FakeHTTPServer{}

		dbSettings, err := settings.NewDatabaseSetting(fakeKeychainAccess, fakeHTTPServer)

		assert.NoError(t, err)
		assert.NotNil(t, dbSettings)
		assert.Equal(t, "/test/directory", dbSettings.Directory)
		assert.Equal(t, "testfile.kdbx", dbSettings.FileName)
		assert.Equal(t, "testpassword", dbSettings.Password)
		assert.Equal(t, "remote_copy", dbSettings.RemoteCopyPrefix)
		assert.Equal(t, "tmp.kdbx", dbSettings.SyncDBName)
		assert.Equal(t, "/test/directory/backups", dbSettings.BackupDirectory)
	})

	t.Run("error when GetPassword fails", func(t *testing.T) {
		os.Setenv("KEEPASS_DB_DIRECTORY", "/test/directory")
		os.Setenv("KEEPASS_DB_FILE_NAME", "testfile.kdbx")
		defer os.Unsetenv("KEEPASS_DB_DIRECTORY")
		defer os.Unsetenv("KEEPASS_DB_FILE_NAME")

		fakeKeychainAccess := &FakeKeychainAccess{password: "", err: errors.New("password retrieval error")}
		fakeHTTPServer := &FakeHTTPServer{}

		dbSettings, err := settings.NewDatabaseSetting(fakeKeychainAccess, fakeHTTPServer)

		assert.Error(t, err)
		assert.Nil(t, dbSettings)
		assert.Equal(t, "can't get keepass db password: password retrieval error", err.Error())
	})

	t.Run("error when KEEPASS_DB_DIRECTORY is not set", func(t *testing.T) {
		os.Unsetenv("KEEPASS_DB_DIRECTORY")
		os.Setenv("KEEPASS_DB_FILE_NAME", "testfile.kdbx")
		defer os.Unsetenv("KEEPASS_DB_FILE_NAME")

		fakeKeychainAccess := &FakeKeychainAccess{password: "testpassword", err: nil}
		fakeHTTPServer := &FakeHTTPServer{}

		dbSettings, err := settings.NewDatabaseSetting(fakeKeychainAccess, fakeHTTPServer)

		assert.Error(t, err)
		assert.Nil(t, dbSettings)
		assert.Equal(t, "can't find directory variable", err.Error())
	})

	t.Run("error when KEEPASS_DB_FILE_NAME is not set", func(t *testing.T) {
		os.Setenv("KEEPASS_DB_DIRECTORY", "/test/directory")
		os.Unsetenv("KEEPASS_DB_FILE_NAME")
		defer os.Unsetenv("KEEPASS_DB_DIRECTORY")

		fakeKeychainAccess := &FakeKeychainAccess{password: "testpassword", err: nil}
		fakeHTTPServer := &FakeHTTPServer{}

		dbSettings, err := settings.NewDatabaseSetting(fakeKeychainAccess, fakeHTTPServer)

		assert.Error(t, err)
		assert.Nil(t, dbSettings)
		assert.Equal(t, "can't find db file name variable", err.Error())
	})
}
