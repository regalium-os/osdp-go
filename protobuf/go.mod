module github.com/regalium-os/osdp-go/protobuf

go 1.27.0

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.2
	capnproto.org/go/capnp/v3 v3.1.0-alpha.2
	github.com/google/flatbuffers v25.12.19+incompatible
	google.golang.org/genproto/googleapis/api v0.0.0-20260911204522-f61a6ca850bd
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260904194346-d0f1323225a4 // indirect
)

// The library this schema module converts for. record and service both import
// it, so without this the module builds only inside go.work and not at all for
// anyone who fetches it.
//
// The replace is what makes v0.0.0 resolvable: the root module carries no tags
// yet. Drop both lines once it is released and this becomes an ordinary
// version requirement.
require github.com/regalium-os/osdp-go v0.0.0

replace github.com/regalium-os/osdp-go => ../
