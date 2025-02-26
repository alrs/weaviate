//                           _       _
// __      _____  __ ___   ___  __ _| |_ ___
// \ \ /\ / / _ \/ _` \ \ / / |/ _` | __/ _ \
//  \ V  V /  __/ (_| |\ V /| | (_| | ||  __/
//   \_/\_/ \___|\__,_| \_/ |_|\__,_|\__\___|
//
//  Copyright © 2016 - 2023 Weaviate B.V. All rights reserved.
//
//  CONTACT: hello@weaviate.io
//

//go:build integrationTest
// +build integrationTest

package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-openapi/strfmt"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate/adapters/repos/db/inverted"
	"github.com/weaviate/weaviate/entities/additional"
	"github.com/weaviate/weaviate/entities/models"
	"github.com/weaviate/weaviate/entities/schema"
	"github.com/weaviate/weaviate/entities/storagestate"
	"github.com/weaviate/weaviate/entities/storobj"
	"github.com/weaviate/weaviate/entities/vectorindex/hnsw"
)

func TestIndex_DropIndex(t *testing.T) {
	dirName := t.TempDir()
	class := &models.Class{Class: "deletetest"}
	index := emptyIdx(t, dirName, class)

	indexFilesBeforeDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	err = index.drop()
	require.Nil(t, err)

	indexFilesAfterDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	assert.Equal(t, 5, len(indexFilesBeforeDelete))
	assert.Equal(t, 0, len(indexFilesAfterDelete))
}

func TestIndex_DropEmptyAndRecreateEmptyIndex(t *testing.T) {
	dirName := t.TempDir()
	class := &models.Class{Class: "deletetest"}
	index := emptyIdx(t, dirName, class)

	indexFilesBeforeDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	// drop the index
	err = index.drop()
	require.Nil(t, err)

	indexFilesAfterDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	index = emptyIdx(t, dirName, class)

	indexFilesAfterRecreate, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	assert.Equal(t, 5, len(indexFilesBeforeDelete))
	assert.Equal(t, 0, len(indexFilesAfterDelete))
	assert.Equal(t, 5, len(indexFilesAfterRecreate))

	err = index.drop()
	require.Nil(t, err)
}

func TestIndex_DropWithDataAndRecreateWithDataIndex(t *testing.T) {
	dirName := t.TempDir()
	logger, _ := test.NewNullLogger()
	class := &models.Class{
		Class: "deletetest",
		Properties: []*models.Property{
			{
				Name:     "name",
				DataType: []string{"string"},
			},
		},
		InvertedIndexConfig: &models.InvertedIndexConfig{},
	}
	fakeSchema := schema.Schema{
		Objects: &models.Schema{
			Classes: []*models.Class{
				class,
			},
		},
	}
	// create index with data
	shardState := singleShardState()
	index, err := NewIndex(testCtx(), IndexConfig{
		RootPath:  dirName,
		ClassName: schema.ClassName(class.Class),
	}, shardState, inverted.ConfigFromModel(class.InvertedIndexConfig),
		hnsw.NewDefaultUserConfig(), &fakeSchemaGetter{
			schema: fakeSchema, shardState: shardState,
		}, nil, logger, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	productsIds := []strfmt.UUID{
		"1295c052-263d-4aae-99dd-920c5a370d06",
		"1295c052-263d-4aae-99dd-920c5a370d07",
	}

	products := []map[string]interface{}{
		{"name": "one"},
		{"name": "two"},
	}

	err = index.addUUIDProperty(context.TODO())
	require.Nil(t, err)

	err = index.addProperty(context.TODO(), &models.Property{
		Name:     "name",
		DataType: []string{"string"},
	})
	require.Nil(t, err)

	for i, p := range products {
		product := models.Object{
			Class:      class.Class,
			ID:         productsIds[i],
			Properties: p,
		}

		err := index.putObject(context.TODO(), storobj.FromObject(
			&product, []float32{0.1, 0.2, 0.01, 0.2}), nil)
		require.Nil(t, err)
	}

	indexFilesBeforeDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	beforeDeleteObj1, err := index.objectByID(context.TODO(),
		productsIds[0], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	beforeDeleteObj2, err := index.objectByID(context.TODO(),
		productsIds[1], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	// drop the index
	err = index.drop()
	require.Nil(t, err)

	indexFilesAfterDelete, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	// recreate the index
	index, err = NewIndex(testCtx(), IndexConfig{
		RootPath:  dirName,
		ClassName: schema.ClassName(class.Class),
	}, shardState, inverted.ConfigFromModel(class.InvertedIndexConfig),
		hnsw.NewDefaultUserConfig(), &fakeSchemaGetter{
			schema:     fakeSchema,
			shardState: shardState,
		}, nil, logger, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = index.addUUIDProperty(context.TODO())
	require.Nil(t, err)
	err = index.addProperty(context.TODO(), &models.Property{
		Name:     "name",
		DataType: []string{"string"},
	})
	require.Nil(t, err)

	indexFilesAfterRecreate, err := getIndexFilenames(dirName, class.Class)
	require.Nil(t, err)

	afterRecreateObj1, err := index.objectByID(context.TODO(),
		productsIds[0], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	afterRecreateObj2, err := index.objectByID(context.TODO(),
		productsIds[1], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	// insert some data in the recreated index
	for i, p := range products {
		thing := models.Object{
			Class:      class.Class,
			ID:         productsIds[i],
			Properties: p,
		}

		err := index.putObject(context.TODO(), storobj.FromObject(
			&thing, []float32{0.1, 0.2, 0.01, 0.2}), nil)
		require.Nil(t, err)
	}

	afterRecreateAndInsertObj1, err := index.objectByID(context.TODO(),
		productsIds[0], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	afterRecreateAndInsertObj2, err := index.objectByID(context.TODO(),
		productsIds[1], nil, additional.Properties{}, nil)
	require.Nil(t, err)

	assert.Equal(t, 5, len(indexFilesBeforeDelete))
	assert.Equal(t, 0, len(indexFilesAfterDelete))
	assert.Equal(t, 5, len(indexFilesAfterRecreate))
	assert.Equal(t, indexFilesBeforeDelete, indexFilesAfterRecreate)
	assert.NotNil(t, beforeDeleteObj1)
	assert.NotNil(t, beforeDeleteObj2)
	assert.Empty(t, afterRecreateObj1)
	assert.Empty(t, afterRecreateObj2)
	assert.NotNil(t, afterRecreateAndInsertObj1)
	assert.NotNil(t, afterRecreateAndInsertObj2)
}

func TestIndex_DropReadOnlyEmptyIndex(t *testing.T) {
	ctx := testCtx()
	class := &models.Class{Class: "deletetest"}
	shard, index := testShard(t, ctx, class.Class)

	err := index.updateShardStatus(ctx, shard.name, storagestate.StatusReadOnly.String())
	require.Nil(t, err)

	err = index.drop()
	require.Nil(t, err)
}

func TestIndex_DropReadOnlyIndexWithData(t *testing.T) {
	ctx := testCtx()
	dirName := t.TempDir()
	logger, _ := test.NewNullLogger()
	class := &models.Class{
		Class: "deletetest",
		Properties: []*models.Property{
			{
				Name:     "name",
				DataType: []string{"string"},
			},
		},
		InvertedIndexConfig: &models.InvertedIndexConfig{},
	}
	fakeSchema := schema.Schema{
		Objects: &models.Schema{
			Classes: []*models.Class{
				class,
			},
		},
	}

	shardState := singleShardState()
	index, err := NewIndex(ctx, IndexConfig{
		RootPath:  dirName,
		ClassName: schema.ClassName(class.Class),
	}, shardState, inverted.ConfigFromModel(class.InvertedIndexConfig),
		hnsw.NewDefaultUserConfig(), &fakeSchemaGetter{
			schema: fakeSchema, shardState: shardState,
		}, nil, logger, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	productsIds := []strfmt.UUID{
		"1295c052-263d-4aae-99dd-920c5a370d06",
		"1295c052-263d-4aae-99dd-920c5a370d07",
	}

	products := []map[string]interface{}{
		{"name": "one"},
		{"name": "two"},
	}

	err = index.addUUIDProperty(ctx)
	require.Nil(t, err)

	err = index.addProperty(ctx, &models.Property{
		Name:     "name",
		DataType: []string{"string"},
	})
	require.Nil(t, err)

	for i, p := range products {
		product := models.Object{
			Class:      class.Class,
			ID:         productsIds[i],
			Properties: p,
		}

		err := index.putObject(ctx, storobj.FromObject(
			&product, []float32{0.1, 0.2, 0.01, 0.2}), nil)
		require.Nil(t, err)
	}

	// set all shards to readonly
	for _, shard := range index.Shards {
		err = shard.updateStatus(storagestate.StatusReadOnly.String())
		require.Nil(t, err)
	}

	err = index.drop()
	require.Nil(t, err)
}

func emptyIdx(t *testing.T, rootDir string, class *models.Class) *Index {
	logger, _ := test.NewNullLogger()
	shardState := singleShardState()

	idx, err := NewIndex(testCtx(), IndexConfig{
		RootPath: rootDir, ClassName: schema.ClassName(class.Class),
	}, shardState, inverted.ConfigFromModel(invertedConfig()),
		hnsw.NewDefaultUserConfig(), &fakeSchemaGetter{
			shardState: shardState,
		}, nil, logger, nil, nil, nil, nil, class, nil)
	require.Nil(t, err)
	return idx
}

func invertedConfig() *models.InvertedIndexConfig {
	return &models.InvertedIndexConfig{
		CleanupIntervalSeconds: 60,
		Stopwords: &models.StopwordConfig{
			Preset: "none",
		},
		IndexNullState:      true,
		IndexPropertyLength: true,
	}
}

func getIndexFilenames(dirName string, className string) ([]string, error) {
	filenames := []string{}
	infos, err := os.ReadDir(dirName)
	if err != nil {
		return filenames, err
	}
	for _, i := range infos {
		if strings.Contains(i.Name(), className) {
			filenames = append(filenames, i.Name())
		}
	}
	return filenames, nil
}
