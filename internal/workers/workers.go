package workers

import (
	"context"
	"fmt"
	"github.com/nikitych1w/softpro-task/internal/config"
	"github.com/nikitych1w/softpro-task/internal/model"
	"github.com/nikitych1w/softpro-task/pkg/store"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type worker struct {
	url     string
	updTime int
}

type BackgroundWorkers struct {
	workers []worker
	cache   *store.Store
	logger  *logrus.Logger
	cntr    int
	done    chan struct{}
}

func New(cfg *config.Config, lg *logrus.Logger, cache *store.Store, w []model.Sport) *BackgroundWorkers {
	var bckWorkers BackgroundWorkers
	bckWorkers.cache = cache
	bckWorkers.logger = lg
	bckWorkers.done = make(chan struct{})

	for _, el := range w {
		var updTime int
		var url string

		switch el {
		case model.Soccer:
			updTime = cfg.Request.UpdateIntervalSoccer
		case model.Football:
			updTime = cfg.Request.UpdateIntervalFootball
		case model.Baseball:
			updTime = cfg.Request.UpdateIntervalBaseball
		}

		url = fmt.Sprintf("%s/%s", cfg.LineProvider.URL, el.String())

		bckWorkers.workers = append(bckWorkers.workers, worker{
			url:     url,
			updTime: updTime,
		})

		logrus.Infof("url: [%s]; updTime: [%ds]", url, updTime)
	}

	return &bckWorkers
}

func (w *BackgroundWorkers) gen() <-chan worker {
	out := make(chan worker)
	go func() {
		for _, el := range w.workers {
			out <- el
		}
		close(out)
	}()
	return out
}

func (w *BackgroundWorkers) getRate(in <-chan worker) <-chan model.Rate {
	out := make(chan model.Rate)
	go func() {

		for wrk := range in {
			for {
				rate := model.Rate{}
				resp, _ := http.Get(wrk.url)
				body, _ := ioutil.ReadAll(resp.Body)
				rate.UnmarshalJSON(body)
				resp.Body.Close()
				out <- rate
				time.Sleep(time.Duration(wrk.updTime) * time.Second)
			}
		}

		close(out)
	}()

	return out
}

func merge(rates ...<-chan model.Rate) <-chan model.Rate {
	var wg sync.WaitGroup
	out := make(chan model.Rate)

	wg.Add(len(rates))
	for _, rt := range rates {
		go func(c <-chan model.Rate) {
			defer wg.Done()
			for n := range c {
				out <- n
			}
		}(rt)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func (w *BackgroundWorkers) RunWorkers() error {
	var channels []<-chan model.Rate
	in := w.gen()

	for range w.workers {
		channels = append(channels, w.getRate(in))
	}

	for n := range merge(channels...) {
		select {
		case <-w.done:
			return nil
		default:
			if err := w.cache.Set(n.RateType.String(), n.RateValue); err != nil {
				return fmt.Errorf("cache set error | [%s]", err)
			}
			w.cntr++
		}
	}

	return nil
}

func (w *BackgroundWorkers) Shutdown(ctx context.Context) error {
	w.done <- struct{}{}
	w.logger.Infof("		========= [workers are stopping...]")
	return nil
}
