package searchrefiner

import (
	"fmt"
	"github.com/boltdb/bolt"
	"os"
	"path"
	"strings"
)

type PluginStorage struct {
	db     *bolt.DB
	plugin string
}

const PluginStoragePath = "plugin_storage"

func (p *PluginStorage) Close() error {
	return p.db.Close()
}

func OpenPluginStorage(plugin string) (*PluginStorage, error) {
	err := os.MkdirAll(PluginStoragePath, 0664)
	if err != nil {
		return nil, nil
	}
	db, err := bolt.Open(path.Join(PluginStoragePath, plugin), 0664, nil)
	return &PluginStorage{
		db:     db,
		plugin: plugin,
	}, nil
}

func (p *PluginStorage) PutValue(bucket, key, value string) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), []byte(value))
	})
}

func (p *PluginStorage) CreateBucket(bucket string) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucket))
		return err
	})
}

func (p *PluginStorage) DeleteKey(bucket, key string) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

func (p *PluginStorage) GetValue(bucket, key string) (string, error) {
	var v []byte
	err := p.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v = b.Get([]byte(key))
		return nil
	})
	return string(v), err
}

func (p *PluginStorage) GetBuckets() ([]string, error) {
	var bu []string
	err := p.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			bu = append(bu, string(name))
			return nil
		})
	})
	return bu, err
}

func (p *PluginStorage) GetValues(bucket string) (map[string]string, error) {
	vals := make(map[string]string)
	err := p.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			vals[string(k)] = string(v)
			return nil
		})
	})
	return vals, err
}

func (p *PluginStorage) ToCSV(bucket string) (string, error) {
	vals, err := p.GetValues(bucket)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for k, v := range vals {
		b.WriteString(fmt.Sprintf("%s,%s\n", k, v))
	}
	return b.String(), nil
}
