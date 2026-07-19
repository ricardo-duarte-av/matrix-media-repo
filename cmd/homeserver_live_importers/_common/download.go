package _common

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/sirupsen/logrus"
	"github.com/t2bot/matrix-media-repo/common/logging"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/database"
	"github.com/t2bot/matrix-media-repo/datastores"
	"github.com/t2bot/matrix-media-repo/homeserver_interop"
	"github.com/t2bot/matrix-media-repo/pipelines/pipeline_upload"
)

type MediaMetadata struct {
	MediaId        string
	ContentType    string
	FileName       string
	UploaderUserId string
	SizeBytes      int64
}

func PsqlMatrixDownloadCopy[M homeserver_interop.ImportDbMedia](ctx rcontext.RequestContext, cfg *ImportOptsPsqlFlatFile, db homeserver_interop.ImportDb[M], extractFn func(record *M) (*MediaMetadata, error)) {
	ctx.Log.Debug("Fetching all local media records from homeserver...")
	records, err := db.GetAllMedia()
	if err != nil {
		panic(err)
	}

	ctx.Log.Info(fmt.Sprintf("Downloading %d media records", len(records)))

	pool, err := ants.NewPool(cfg.NumWorkers, ants.WithOptions(ants.Options{
		ExpiryDuration:   1 * time.Hour,
		PreAlloc:         false,
		MaxBlockingTasks: 0, // no limit
		Nonblocking:      false,
		Logger:           &logging.SendToDebugLogger{},
		DisablePurge:     false,
		PanicHandler: func(err interface{}) {
			panic(err)
		},
	}))
	if err != nil {
		panic(err)
	}

	numCompleted := 0
	numFailed := 0
	mu := &sync.RWMutex{}
	onComplete := func() {
		mu.Lock()
		numCompleted++
		percent := int((float32(numCompleted) / float32(len(records))) * 100)
		ctx.Log.Info(fmt.Sprintf("%d/%d processed (%d%%)", numCompleted, len(records), percent))
		mu.Unlock()
	}
	onFail := func() {
		mu.Lock()
		numFailed++
		mu.Unlock()
	}

	for i := 0; i < len(records); i++ {
		percent := int((float32(i+1) / float32(len(records))) * 100)
		record := records[i]

		meta, err := extractFn(record)
		if err != nil {
			panic(err)
		}

		ctx.Log.Debug(fmt.Sprintf("Queuing %s (%d/%d %d%%)", meta.MediaId, i+1, len(records), percent))
		err = pool.Submit(doWork(ctx, meta, cfg.ServerName, cfg.ApiUrl, cfg.AccessToken, onComplete, onFail))
		if err != nil {
			panic(err)
		}
	}

	for {
		mu.RLock()
		done := numCompleted
		mu.RUnlock()
		if done >= len(records) {
			break
		}
		ctx.Log.Debug("Waiting for import to complete...")
		time.Sleep(1 * time.Second)
	}

	if numFailed > 0 {
		ctx.Log.Warnf("Import completed with %d/%d records failed (see warnings above). Re-running will retry only the failed/missing media.", numFailed, len(records))
	} else {
		ctx.Log.Info("Import completed")
	}
}

func doWork(ctx rcontext.RequestContext, record *MediaMetadata, serverName string, csApiUrl string, accessToken string, onComplete func(), onFail func()) func() {
	return func() {
		defer onComplete()

		ctx := ctx.LogWithFields(logrus.Fields{"origin": serverName, "mediaId": record.MediaId})

		db := database.GetInstance().Media.Prepare(ctx)

		dbRecord, err := db.GetById(serverName, record.MediaId)
		if err != nil {
			panic(err) // a failure to read our own database is systemic, not per-item
		}
		if dbRecord != nil {
			ctx.Log.Debug("Already downloaded - skipping")
			return
		}

		body, err := downloadMedia(csApiUrl, serverName, record.MediaId, accessToken)
		if err != nil {
			ctx.Log.Warnf("Failed to download - skipping: %v", err)
			onFail()
			return
		}

		dbRecord, err = pipeline_upload.Execute(ctx, serverName, record.MediaId, body, record.ContentType, record.FileName, record.UploaderUserId, datastores.LocalMediaKind)
		if err != nil {
			ctx.Log.Warnf("Failed to import - skipping: %v", err)
			onFail()
			return
		}

		if dbRecord.SizeBytes != record.SizeBytes {
			ctx.Log.Warnf("Size mismatch! Expected %d bytes but got %d", record.SizeBytes, dbRecord.SizeBytes)
		}
	}
}

func downloadMedia(baseUrl string, serverName string, mediaId string, accessToken string) (io.ReadCloser, error) {
	var downloadUrl string
	if accessToken != "" {
		// Authenticated media (MSC3916): requires any valid access token. Encrypted (E2EE)
		// media is served the same way - as an opaque blob - so no special handling is needed.
		downloadUrl = baseUrl + "/_matrix/client/v1/media/download/" + serverName + "/" + mediaId
	} else {
		downloadUrl = baseUrl + "/_matrix/media/v3/download/" + serverName + "/" + mediaId
	}

	req, err := http.NewRequest(http.MethodGet, downloadUrl, nil)
	if err != nil {
		return nil, err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, errors.New("received status code " + strconv.Itoa(resp.StatusCode))
	}

	return resp.Body, nil
}
