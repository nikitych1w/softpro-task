package rpcserver

import (
	"context"
	"fmt"
	"github.com/nikitych1w/softpro-task/pkg/store"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type RPCServer struct {
	listener net.Listener
	srv      *grpc.Server
	logger   *logrus.Logger
	store    *store.Store
	prevResp map[string]float32
	mtx      *sync.Mutex
	wg       *sync.WaitGroup
}

type reqParams struct {
	sportsToUpdate []string
	updTime        int
}

func NewRPCServer(lg *logrus.Logger, str *store.Store) *RPCServer {
	var s RPCServer

	s.logger = lg
	s.store = str
	s.prevResp = make(map[string]float32)
	s.mtx = &sync.Mutex{}
	s.srv = grpc.NewServer()
	s.wg = &sync.WaitGroup{}
	RegisterLineProcessorServer(s.srv, &s)
	reflection.Register(s.srv)

	return &s
}

func (s *RPCServer) ListenAndServe(url string, ctx context.Context) error {
	log.Println("rpc start on", url)
	var err error
	s.listener, err = net.Listen("tcp", url)
	if err != nil {
		logrus.Error(err)
	}

	return s.srv.Serve(s.listener)
}

type rawToDelta struct {
	raw, delta float32
}
type requestAndPreviousDelta struct {
	req  *Request
	prev map[string]rawToDelta
}

func (s *RPCServer) process(stream LineProcessor_SubscribeOnSportsLinesServer) error {
	subscribeRequests := make(chan requestAndPreviousDelta)
	prevResp := make(map[string]rawToDelta)

	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				fmt.Println("GRPC stream: (EOF)")
			}
			if err != nil {
				fmt.Println("GRPC stream:", err)
			}

			subscribeRequests <- requestAndPreviousDelta{
				req:  in,
				prev: prevResp,
			}
		}
	}()

	for request := range subscribeRequests {
		var val int
		var err error

		rp := reqParams{}
		rp.sportsToUpdate = request.req.GetSports()

		if val, err = strconv.Atoi(request.req.GetTimeUpd()); err != nil {
			s.logger.Errorf("GRPC stream: (can'requestAndPreviousDelta convert interval value | [%s])", err.Error())
		}
		rp.updTime = val

		s.wg.Add(1)
		go func(rp reqParams, prevResp map[string]rawToDelta) {
			defer s.wg.Done()
			for range time.Tick(time.Duration(rp.updTime) * time.Second) {
				data := s.buildResponse(rp, prevResp)
				respData := make(map[string]float32)
				for k, v := range data {
					respData[k] = v.delta
				}

				if err := stream.Send(&Response{Line: respData}); err != nil {
					s.logger.Errorf("GRPC stream: (streaming error | [%s])", err.Error())
				}

				s.logger.Info("\t ---> [GRPC] : SENT TO STREAM ", rp, data)

				s.mtx.Lock()
				prevResp = data
				s.mtx.Unlock()
			}
		}(rp, request.prev)
	}

	s.wg.Wait()

	return nil
}

func (s *RPCServer) buildResponse(rp reqParams, prevResp map[string]rawToDelta) map[string]rawToDelta {
	logrus.Println(prevResp)
	currResp := make(map[string]rawToDelta)
	var prevKeys []string

	if len(prevResp) > 0 {
		for k, _ := range prevResp {
			prevKeys = append(prevKeys, k)
		}
	}

	for _, el := range rp.sportsToUpdate {
		val, err := s.store.GetLastValueByKey(el)
		if err != nil {
			s.logger.Errorf("GRPC stream: (getting from store error | [%s])", err.Error())
		}

		var res float32
		if len(prevResp) > 0 && compareSlice(prevKeys, rp.sportsToUpdate) {
			res = val - prevResp[el].delta
		} else {
			res = val
		}

		s.mtx.Lock()
		currResp[el] = rawToDelta{
			raw:   val,
			delta: res,
		}
		s.mtx.Unlock()
	}

	return currResp
}

func compareSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func (s *RPCServer) SubscribeOnSportsLines(stream LineProcessor_SubscribeOnSportsLinesServer) error {
	s.process(stream)
	return nil
}
