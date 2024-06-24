package keepass_test

import (
	"bytes"
	"testing"

	"kdbxsync/keepass"
	"kdbxsync/settings"

	"github.com/stretchr/testify/assert"
	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

// storage fake
type fakeStorage struct{}

func (storage *fakeStorage) UpdateDBFile() error {
	return nil
}

func (storage *fakeStorage) DownloadRemoteKeepassDB() error {
	return nil
}

func (storage *fakeStorage) BackupDBFile() error {
	return nil
}

// http server fake
type FakeHTTPServer struct{}

func (fhs *FakeHTTPServer) RunHTTPServer() {}
func (fhs *FakeHTTPServer) ReadChannels() (string, error) {
	return "pass", nil
}

// fake keepass database
func mkValue(key string, value string) gokeepasslib.ValueData {
	return gokeepasslib.ValueData{Key: key, Value: gokeepasslib.V{Content: value}}
}

func mkProtectedValue(key string, value string) gokeepasslib.ValueData {
	return gokeepasslib.ValueData{
		Key:   key,
		Value: gokeepasslib.V{Content: value, Protected: w.NewBoolWrapper(true)},
	}
}

func newFakeKeepassDatabase() *gokeepasslib.Database {
	rootGroup := gokeepasslib.NewGroup()
	rootGroup.Name = "Root"

	entry := gokeepasslib.NewEntry()
	entry.Values = append(entry.Values, mkValue("Title", "My pass"))
	entry.Values = append(entry.Values, mkValue("UserName", "example@mail.com"))
	entry.Values = append(entry.Values, mkProtectedValue("Password", "pass1"))

	rootGroup.Entries = append(rootGroup.Entries, entry)

	keepassBase := &gokeepasslib.Database{
		Header:      gokeepasslib.NewHeader(),
		Credentials: gokeepasslib.NewPasswordCredentials("pass"),
		Content: &gokeepasslib.DBContent{
			Meta: gokeepasslib.NewMetaData(),
			Root: &gokeepasslib.RootData{
				Groups: []gokeepasslib.Group{rootGroup},
			},
		},
	}

	return keepassBase
}

func TestNewKeepassDBSync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		localDBFileObj := &bytes.Buffer{}
		remoteDBCopyFileObj := &bytes.Buffer{}
		tmpSyncFileObj := &bytes.Buffer{}

		keepassBase := newFakeKeepassDatabase()

		gokeepasslib.NewEncoder(localDBFileObj).Encode(keepassBase)
		gokeepasslib.NewEncoder(remoteDBCopyFileObj).Encode(keepassBase)
		gokeepasslib.NewEncoder(tmpSyncFileObj).Encode(keepassBase)

		storage := &fakeStorage{}
		dbSettings := settings.DataBaseSettings{
			Directory:        "/test/directory",
			FileName:         "testfile.kdbx",
			Password:         "pass",
			RemoteCopyPrefix: "remote",
			SyncDBName:       "tmp.kdbx",
			BackupDirectory:  "backups",
		}
		settings := &settings.AppSettings{
			HTTPServer:         &FakeHTTPServer{},
			DatabaseSettings:   &dbSettings,
			StorageCredentials: "pass",
		}

		dbSync, err := keepass.NewKeepassDBSync(localDBFileObj, remoteDBCopyFileObj, tmpSyncFileObj, storage, settings)

		assert.NoError(t, err)
		assert.NotNil(t, dbSync)
	})

	t.Run("error: wrong keepass database password", func(t *testing.T) {
		localDBFileObj := &bytes.Buffer{}
		remoteDBCopyFileObj := &bytes.Buffer{}
		tmpSyncFileObj := &bytes.Buffer{}

		keepassBase := newFakeKeepassDatabase()

		gokeepasslib.NewEncoder(localDBFileObj).Encode(keepassBase)
		gokeepasslib.NewEncoder(remoteDBCopyFileObj).Encode(keepassBase)
		gokeepasslib.NewEncoder(tmpSyncFileObj).Encode(keepassBase)

		storage := &fakeStorage{}
		dbSettings := settings.DataBaseSettings{
			Directory:        "/test/directory",
			FileName:         "testfile.kdbx",
			Password:         "",
			RemoteCopyPrefix: "remote",
			SyncDBName:       "tmp.kdbx",
			BackupDirectory:  "backups",
		}
		settings := &settings.AppSettings{
			HTTPServer:         &FakeHTTPServer{},
			DatabaseSettings:   &dbSettings,
			StorageCredentials: "pass",
		}

		dbSync, err := keepass.NewKeepassDBSync(localDBFileObj, remoteDBCopyFileObj, tmpSyncFileObj, storage, settings)

		assert.Error(t, err)
		assert.Nil(t, dbSync)
		assert.Equal(
			t,
			"can't initialize local Keepass DB: Wrong password? Database integrity check failed",
			err.Error(),
		)
	})
}
