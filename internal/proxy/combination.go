package proxy

import (
	"context"
	"encoding/json"
	"errors"
	// "fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/common/models"
)

var (
	// ErrTimeout ...
	ErrTimeout = errors.New("combineReq timeout error")

	// TODO: support config
	// client to request multi server timeout duration, default is 5 second
	combinationClientReqTimeout = 5 * time.Second
)

type responseChan struct {
	Err   error
	Field string
	Data  map[string]interface{}
}

func combineReq(ctx context.Context, serverHost string, body io.Reader,
	cfg *models.APICombination, rc chan<- responseChan) {
	var (
		err error
		r   = responseChan{
			Err:   nil,
			Field: cfg.Field,
			Data:  nil,
		}
	)
	// logger.Logger.Info("combineReq calling")

	defer func() {
		if v := recover(); v != nil {
			logger.Logger.Errorf("[proxier] combineReq panic: %s", debug.Stack())
			err = v.(error)
			r.Err = err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Logger.Info("[proxier] combineReq timeout!")
		r.Err = ErrTimeout
		break
	default:
		var (
			req  *http.Request
			resp *http.Response
			err  error
		)
		// prepare request with method, path and data from body
		if req, err = http.NewRequest(cfg.Method, serverHost+cfg.Path, body); err != nil {
			r.Err = err
			logger.Logger.Errorf("could not finish NewRequest: %v", err)
			break

		}
		// send to server
		client := http.Client{Timeout: combinationClientReqTimeout}
		if resp, err = client.Do(req); err != nil {
			r.Err = err
			logger.Logger.Errorf("could not finish client.Do: %v", err)
			break
		}

		// read response
		byts, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			r.Err = err
			logger.Logger.Errorf("could not ReadAll resp.Body: %v", err)
			break
		}
		defer resp.Body.Close()
		json.Unmarshal(byts, &r.Data)
	}

	// put response into channel
	rc <- r
}
