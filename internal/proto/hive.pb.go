package proto

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Chunk represents a single document chunk ingested into the Hive.
type Chunk struct {
	Id         string
	DocumentId string
	Content    string
	Vector     []float32
	Metadata   map[string]string
}

// Status communicates ingestion status back to the drone client.
type Status struct {
	Success bool
	Message string
	ChunkId string
}

// Search represents an end-user query against the Hive.
type Search struct {
	Query       string
	TopK        int32
	QueryVector []float32
}

// Match is a single search hit returned to the UI.
type Match struct {
	ChunkId    string
	DocumentId string
	Content    string
	Score      float32
	Metadata   map[string]string
}

// Result aggregates a list of matches.
type Result struct {
	Matches []*Match
}

// HiveClient is the client-side gRPC API.
type HiveClient interface {
	Ingest(ctx context.Context, in *Chunk, opts ...grpc.CallOption) (*Status, error)
	Query(ctx context.Context, in *Search, opts ...grpc.CallOption) (*Result, error)
}

type hiveClient struct {
	cc grpc.ClientConnInterface
}

// NewHiveClient constructs a new gRPC client.
func NewHiveClient(cc grpc.ClientConnInterface) HiveClient {
	return &hiveClient{cc: cc}
}

func (c *hiveClient) Ingest(ctx context.Context, in *Chunk, opts ...grpc.CallOption) (*Status, error) {
	out := new(Status)
	err := c.cc.Invoke(ctx, "/hive.Hive/Ingest", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hiveClient) Query(ctx context.Context, in *Search, opts ...grpc.CallOption) (*Result, error) {
	out := new(Result)
	err := c.cc.Invoke(ctx, "/hive.Hive/Query", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// HiveServer is the server-side gRPC API.
type HiveServer interface {
	Ingest(context.Context, *Chunk) (*Status, error)
	Query(context.Context, *Search) (*Result, error)
	mustEmbedUnimplementedHiveServer()
}

// UnimplementedHiveServer can be embedded to have forward compatible implementations.
type UnimplementedHiveServer struct{}

func (UnimplementedHiveServer) Ingest(context.Context, *Chunk) (*Status, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Ingest not implemented")
}

func (UnimplementedHiveServer) Query(context.Context, *Search) (*Result, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Query not implemented")
}

func (UnimplementedHiveServer) mustEmbedUnimplementedHiveServer() {}

// RegisterHiveServer registers the Hive service with the provided gRPC server registrar.
func RegisterHiveServer(s grpc.ServiceRegistrar, srv HiveServer) {
	s.RegisterService(&Hive_ServiceDesc, srv)
}

// Hive_ServiceDesc describes the Hive service to gRPC.
var Hive_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "hive.Hive",
	HandlerType: (*HiveServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Ingest",
			Handler:    _Hive_Ingest_Handler,
		},
		{
			MethodName: "Query",
			Handler:    _Hive_Query_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "backend/proto/hive.proto",
}

func _Hive_Ingest_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Chunk)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HiveServer).Ingest(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/hive.Hive/Ingest",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(HiveServer).Ingest(ctx, req.(*Chunk))
	}
	return interceptor(ctx, in, info, handler)
}

func _Hive_Query_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Search)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HiveServer).Query(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/hive.Hive/Query",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(HiveServer).Query(ctx, req.(*Search))
	}
	return interceptor(ctx, in, info, handler)
}

