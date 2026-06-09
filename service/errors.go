package service

import (
	"errors"
	"net"
	"net/http"

	"github.com/alcounit/selenosis/v2/pkg/jsonrpc"
	"github.com/alcounit/selenosis/v2/pkg/proxy"
	"github.com/alcounit/selenosis/v2/pkg/selenium"
	"github.com/rs/zerolog"
)

func isUpstreamUnreachable(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func createSessionProxyErrorHandler(log zerolog.Logger, ip string) proxy.ErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		log.Err(err).Str("ip", ip).Msg("session create proxy error")
		writeErrorResponse(w, http.StatusInternalServerError, selenium.ErrSessionNotCreated(err))
	}
}

func sessionProxyErrorHandler(log zerolog.Logger, sessionId string) proxy.ErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if isUpstreamUnreachable(err) {
			log.Warn().Err(err).Str("sessionId", sessionId).Msg("session pod unreachable")
			writeErrorResponse(w, http.StatusNotFound, selenium.ErrInvalidSessionId(err))
			return
		}
		log.Err(err).Str("sessionId", sessionId).Msg("session proxy error")
		writeErrorResponse(w, http.StatusInternalServerError, selenium.ErrUnknown(err))
	}
}

func routeHTTPProxyErrorHandler(log zerolog.Logger, sessionId string) proxy.ErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if isUpstreamUnreachable(err) {
			log.Warn().Err(err).Str("sessionId", sessionId).Msg("pod unreachable")
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		log.Err(err).Str("sessionId", sessionId).Msg("http proxy error")
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func mcpInitProxyErrorHandler(log zerolog.Logger, ip string) proxy.ErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		log.Err(err).Str("ip", ip).Msg("mcp initialize proxy error")
		jsonrpc.WriteError(w, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error")
	}
}

func mcpProxyErrorHandler(log zerolog.Logger, ip string) proxy.ErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if isUpstreamUnreachable(err) {
			log.Warn().Err(err).Str("ip", ip).Msg("mcp upstream unreachable, session expired")
			jsonrpc.WriteError(w, http.StatusNotFound, jsonrpc.SessionNotFound, "Session not found")
			return
		}
		log.Err(err).Str("ip", ip).Msg("mcp proxy error")
		jsonrpc.WriteError(w, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error")
	}
}

type browserError struct {
	kind errorKind
	err  error
}

func writeCreateSessionWaitError(rw http.ResponseWriter, waitErr *browserError) {
	switch waitErr.kind {
	case browserCreate:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.Error("failed to create browser", waitErr.err))
	case browserEventsStart:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.Error("failed to start browser event stream", waitErr.err))
	case browserStreamClosed:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
	case browserFailed:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.Error("browser failed to start", ErrInternal))
	case browserStreamError:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(waitErr.err))
	case browserContextDone:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
	default:
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
	}
}

func writePlaywrightWaitError(rw http.ResponseWriter, waitErr *browserError) {
	switch waitErr.kind {
	case browserCreate:
		http.Error(rw, "failed to create browser resource", http.StatusInternalServerError)
	case browserEventsStart:
		http.Error(rw, "failed to start browser event stream", http.StatusInternalServerError)
	case browserStreamClosed:
		http.Error(rw, "browser event stream closed unexpectedly", http.StatusInternalServerError)
	case browserFailed:
		http.Error(rw, "browser failed to start", http.StatusInternalServerError)
	case browserStreamError:
		http.Error(rw, "browser event stream error", http.StatusInternalServerError)
	case browserContextDone:
		http.Error(rw, "context cancelled, stopping browser event stream", http.StatusInternalServerError)
	default:
		http.Error(rw, "internal server error", http.StatusInternalServerError)
	}
}

func writeMcpWaitError(rw http.ResponseWriter, waitErr *browserError) {
	switch waitErr.kind {
	case browserCreate:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: failed to create browser resource")
	case browserEventsStart:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: failed to start browser event stream")
	case browserStreamClosed:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: browser event stream closed unexpectedly")
	case browserFailed:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: browser failed to start")
	case browserStreamError:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: browser event stream error")
	case browserContextDone:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error: context cancelled, stopping browser event stream")
	default:
		jsonrpc.WriteError(rw, http.StatusInternalServerError, jsonrpc.InternalError, "Internal error")
	}
}
