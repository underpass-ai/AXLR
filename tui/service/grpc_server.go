package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const grpcServiceName = "underpass.axlr.v1.AxlrService"

// The public proto uses BytesValue to carry exact UTF-8 JSON (including uint64
// revisions and tool numbers). Consumers can generate ordinary protobuf clients;
// the server descriptor is derived from the same inventory as HTTP and MCP.
type grpcService interface {
	grpcInvoke(context.Context, operation, []byte, func(Event) error) (*wrapperspb.BytesValue, error)
}

func (s *Server) grpcServer() (*grpc.Server, error) {
	tlsConfig, err := serverTLSConfig(s.Config)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.MaxRecvMsgSize((4<<20)+1024), grpc.MaxSendMsgSize(16<<20))
	desc := grpc.ServiceDesc{ServiceName: grpcServiceName, HandlerType: (*grpcService)(nil), Metadata: "api/proto/underpass/axlr/v1/axlr.proto"}
	for _, op := range operations() {
		if op.Name == "StreamEvents" {
			desc.Streams = append(desc.Streams, grpc.StreamDesc{StreamName: op.Name, ServerStreams: true, Handler: func(srv any, stream grpc.ServerStream) error {
				input := new(wrapperspb.BytesValue)
				if err := stream.RecvMsg(input); err != nil {
					return err
				}
				emit := func(event Event) error {
					body, _ := json.Marshal(map[string]any{"kind": "event", "event": event})
					return stream.SendMsg(wrapperspb.Bytes(body))
				}
				result, err := srv.(grpcService).grpcInvoke(stream.Context(), op, input.Value, emit)
				if err != nil {
					return err
				}
				return stream.SendMsg(result)
			}})
			continue
		}
		desc.Methods = append(desc.Methods, grpc.MethodDesc{MethodName: op.Name, Handler: func(srv any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			input := new(wrapperspb.BytesValue)
			if err := decode(input); err != nil {
				return nil, err
			}
			handler := func(ctx context.Context, req any) (any, error) {
				return srv.(grpcService).grpcInvoke(ctx, op, req.(*wrapperspb.BytesValue).Value, nil)
			}
			if interceptor == nil {
				return handler(ctx, input)
			}
			return interceptor(ctx, input, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + grpcServiceName + "/" + op.Name}, handler)
		}})
	}
	server.RegisterService(&desc, s)
	return server, nil
}

func (s *Server) grpcInvoke(ctx context.Context, op operation, data []byte, emit func(Event) error) (*wrapperspb.BytesValue, error) {
	var state *tls.ConnectionState
	if p, ok := peer.FromContext(ctx); ok {
		if info, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			state = &info.State
		}
	}
	result := s.invokeOperation(ctx, op, data, state, "", emit)
	encoded, _ := json.Marshal(result)
	if result.StatusCode >= 400 {
		code := codes.Internal
		switch result.StatusCode {
		case 400, 415, 422:
			code = codes.InvalidArgument
		case 403:
			code = codes.PermissionDenied
		case 404:
			code = codes.NotFound
		case 409:
			code = codes.FailedPrecondition
		case 413, 429:
			code = codes.ResourceExhausted
		case 503:
			code = codes.Unavailable
		}
		var body struct {
			Error struct{ Code, Message string }
		}
		_ = json.Unmarshal(result.Body, &body)
		st, err := status.New(code, body.Error.Message).WithDetails(&errdetails.ErrorInfo{Reason: body.Error.Code, Domain: grpcServiceName, Metadata: map[string]string{"http_status": strconv.Itoa(result.StatusCode), "result_json": string(encoded)}})
		if err != nil {
			return nil, status.Error(code, body.Error.Message)
		}
		return nil, st.Err()
	}
	return wrapperspb.Bytes(encoded), nil
}
