package server

import (
	"net/http"

	"manifold/internal/a2a/rpc"
)

type Server struct {
	rpc *rpc.Router
}

func NewServer(store TaskStore, auth Authenticator) *Server {
	router := rpc.NewRouter()

	// Register methods
	// router.Register("tasks/send", s.handleSend)
	// router.Register("tasks/sendSubscribe", s.handleSendSubscribe)
	// router.Register("tasks/get", s.handleGet)
	// router.Register("tasks/cancel", s.handleCancel)
	// router.Register("tasks/pushNotification/set", s.handlePushSet)
	// router.Register("tasks/pushNotification/get", s.handlePushGet)
	// router.Register("tasks/resubscribe", s.handleResubscribe)

	return &Server{
		rpc: router,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.rpc.ServeHTTP(w, r)
}
