package reqlimit

import "connectrpc.com/connect"

// HandlerOption is what a module's test harness mounts its handlers with, in
// place of production's connectOptions (cmd/api): the Connect protocol header
// requirement, the field limits (Interceptor) and the backstop of one message.
// Wrap the harness's mux with Middleware too (httpserver.Server.Handler already
// does), so that a test through a real handler is refused as in production.
func HandlerOption() connect.HandlerOption {
	return connect.WithHandlerOptions(
		connect.WithRequireConnectProtocolHeader(),
		connect.WithInterceptors(Interceptor()),
		connect.WithReadMaxBytes(Backstop),
	)
}
