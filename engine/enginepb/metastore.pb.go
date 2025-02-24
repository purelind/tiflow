// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.20.1
// source: engine/proto/metastore.proto

package enginepb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StoreType int32

const (
	StoreType_ServiceDiscovery StoreType = 0
	StoreType_SystemMetaStore  StoreType = 1
	StoreType_AppMetaStore     StoreType = 2
)

// Enum value maps for StoreType.
var (
	StoreType_name = map[int32]string{
		0: "ServiceDiscovery",
		1: "SystemMetaStore",
		2: "AppMetaStore",
	}
	StoreType_value = map[string]int32{
		"ServiceDiscovery": 0,
		"SystemMetaStore":  1,
		"AppMetaStore":     2,
	}
)

func (x StoreType) Enum() *StoreType {
	p := new(StoreType)
	*p = x
	return p
}

func (x StoreType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (StoreType) Descriptor() protoreflect.EnumDescriptor {
	return file_engine_proto_metastore_proto_enumTypes[0].Descriptor()
}

func (StoreType) Type() protoreflect.EnumType {
	return &file_engine_proto_metastore_proto_enumTypes[0]
}

func (x StoreType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use StoreType.Descriptor instead.
func (StoreType) EnumDescriptor() ([]byte, []int) {
	return file_engine_proto_metastore_proto_rawDescGZIP(), []int{0}
}

type RegisterMetaStoreRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Address string    `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`
	Tp      StoreType `protobuf:"varint,2,opt,name=tp,proto3,enum=enginepb.StoreType" json:"tp,omitempty"`
}

func (x *RegisterMetaStoreRequest) Reset() {
	*x = RegisterMetaStoreRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_engine_proto_metastore_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RegisterMetaStoreRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterMetaStoreRequest) ProtoMessage() {}

func (x *RegisterMetaStoreRequest) ProtoReflect() protoreflect.Message {
	mi := &file_engine_proto_metastore_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterMetaStoreRequest.ProtoReflect.Descriptor instead.
func (*RegisterMetaStoreRequest) Descriptor() ([]byte, []int) {
	return file_engine_proto_metastore_proto_rawDescGZIP(), []int{0}
}

func (x *RegisterMetaStoreRequest) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *RegisterMetaStoreRequest) GetTp() StoreType {
	if x != nil {
		return x.Tp
	}
	return StoreType_ServiceDiscovery
}

type RegisterMetaStoreResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Err *Error `protobuf:"bytes,1,opt,name=err,proto3" json:"err,omitempty"`
}

func (x *RegisterMetaStoreResponse) Reset() {
	*x = RegisterMetaStoreResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_engine_proto_metastore_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RegisterMetaStoreResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterMetaStoreResponse) ProtoMessage() {}

func (x *RegisterMetaStoreResponse) ProtoReflect() protoreflect.Message {
	mi := &file_engine_proto_metastore_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterMetaStoreResponse.ProtoReflect.Descriptor instead.
func (*RegisterMetaStoreResponse) Descriptor() ([]byte, []int) {
	return file_engine_proto_metastore_proto_rawDescGZIP(), []int{1}
}

func (x *RegisterMetaStoreResponse) GetErr() *Error {
	if x != nil {
		return x.Err
	}
	return nil
}

type QueryMetaStoreRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Tp StoreType `protobuf:"varint,1,opt,name=tp,proto3,enum=enginepb.StoreType" json:"tp,omitempty"`
}

func (x *QueryMetaStoreRequest) Reset() {
	*x = QueryMetaStoreRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_engine_proto_metastore_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryMetaStoreRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryMetaStoreRequest) ProtoMessage() {}

func (x *QueryMetaStoreRequest) ProtoReflect() protoreflect.Message {
	mi := &file_engine_proto_metastore_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryMetaStoreRequest.ProtoReflect.Descriptor instead.
func (*QueryMetaStoreRequest) Descriptor() ([]byte, []int) {
	return file_engine_proto_metastore_proto_rawDescGZIP(), []int{2}
}

func (x *QueryMetaStoreRequest) GetTp() StoreType {
	if x != nil {
		return x.Tp
	}
	return StoreType_ServiceDiscovery
}

type QueryMetaStoreResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Err     *Error `protobuf:"bytes,1,opt,name=err,proto3" json:"err,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *QueryMetaStoreResponse) Reset() {
	*x = QueryMetaStoreResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_engine_proto_metastore_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryMetaStoreResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryMetaStoreResponse) ProtoMessage() {}

func (x *QueryMetaStoreResponse) ProtoReflect() protoreflect.Message {
	mi := &file_engine_proto_metastore_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryMetaStoreResponse.ProtoReflect.Descriptor instead.
func (*QueryMetaStoreResponse) Descriptor() ([]byte, []int) {
	return file_engine_proto_metastore_proto_rawDescGZIP(), []int{3}
}

func (x *QueryMetaStoreResponse) GetErr() *Error {
	if x != nil {
		return x.Err
	}
	return nil
}

func (x *QueryMetaStoreResponse) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

var File_engine_proto_metastore_proto protoreflect.FileDescriptor

var file_engine_proto_metastore_proto_rawDesc = []byte{
	0x0a, 0x1c, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6d,
	0x65, 0x74, 0x61, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x70, 0x62, 0x1a, 0x18, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0x59, 0x0a, 0x18, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x4d, 0x65,
	0x74, 0x61, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18,
	0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x12, 0x23, 0x0a, 0x02, 0x74, 0x70, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x70, 0x62, 0x2e,
	0x53, 0x74, 0x6f, 0x72, 0x65, 0x54, 0x79, 0x70, 0x65, 0x52, 0x02, 0x74, 0x70, 0x22, 0x3e, 0x0a,
	0x19, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x4d, 0x65, 0x74, 0x61, 0x53, 0x74, 0x6f,
	0x72, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x21, 0x0a, 0x03, 0x65, 0x72,
	0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65,
	0x70, 0x62, 0x2e, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x52, 0x03, 0x65, 0x72, 0x72, 0x22, 0x3c, 0x0a,
	0x15, 0x51, 0x75, 0x65, 0x72, 0x79, 0x4d, 0x65, 0x74, 0x61, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x23, 0x0a, 0x02, 0x74, 0x70, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0e, 0x32, 0x13, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x70, 0x62, 0x2e, 0x53, 0x74,
	0x6f, 0x72, 0x65, 0x54, 0x79, 0x70, 0x65, 0x52, 0x02, 0x74, 0x70, 0x22, 0x55, 0x0a, 0x16, 0x51,
	0x75, 0x65, 0x72, 0x79, 0x4d, 0x65, 0x74, 0x61, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x21, 0x0a, 0x03, 0x65, 0x72, 0x72, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x70, 0x62, 0x2e, 0x45, 0x72,
	0x72, 0x6f, 0x72, 0x52, 0x03, 0x65, 0x72, 0x72, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72,
	0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65,
	0x73, 0x73, 0x2a, 0x48, 0x0a, 0x09, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12,
	0x14, 0x0a, 0x10, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x44, 0x69, 0x73, 0x63, 0x6f, 0x76,
	0x65, 0x72, 0x79, 0x10, 0x00, 0x12, 0x13, 0x0a, 0x0f, 0x53, 0x79, 0x73, 0x74, 0x65, 0x6d, 0x4d,
	0x65, 0x74, 0x61, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x10, 0x01, 0x12, 0x10, 0x0a, 0x0c, 0x41, 0x70,
	0x70, 0x4d, 0x65, 0x74, 0x61, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x10, 0x02, 0x42, 0x2b, 0x5a, 0x29,
	0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x70, 0x69, 0x6e, 0x67, 0x63,
	0x61, 0x70, 0x2f, 0x74, 0x69, 0x66, 0x6c, 0x6f, 0x77, 0x2f, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65,
	0x2f, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_engine_proto_metastore_proto_rawDescOnce sync.Once
	file_engine_proto_metastore_proto_rawDescData = file_engine_proto_metastore_proto_rawDesc
)

func file_engine_proto_metastore_proto_rawDescGZIP() []byte {
	file_engine_proto_metastore_proto_rawDescOnce.Do(func() {
		file_engine_proto_metastore_proto_rawDescData = protoimpl.X.CompressGZIP(file_engine_proto_metastore_proto_rawDescData)
	})
	return file_engine_proto_metastore_proto_rawDescData
}

var file_engine_proto_metastore_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_engine_proto_metastore_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_engine_proto_metastore_proto_goTypes = []interface{}{
	(StoreType)(0),                    // 0: enginepb.StoreType
	(*RegisterMetaStoreRequest)(nil),  // 1: enginepb.RegisterMetaStoreRequest
	(*RegisterMetaStoreResponse)(nil), // 2: enginepb.RegisterMetaStoreResponse
	(*QueryMetaStoreRequest)(nil),     // 3: enginepb.QueryMetaStoreRequest
	(*QueryMetaStoreResponse)(nil),    // 4: enginepb.QueryMetaStoreResponse
	(*Error)(nil),                     // 5: enginepb.Error
}
var file_engine_proto_metastore_proto_depIdxs = []int32{
	0, // 0: enginepb.RegisterMetaStoreRequest.tp:type_name -> enginepb.StoreType
	5, // 1: enginepb.RegisterMetaStoreResponse.err:type_name -> enginepb.Error
	0, // 2: enginepb.QueryMetaStoreRequest.tp:type_name -> enginepb.StoreType
	5, // 3: enginepb.QueryMetaStoreResponse.err:type_name -> enginepb.Error
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_engine_proto_metastore_proto_init() }
func file_engine_proto_metastore_proto_init() {
	if File_engine_proto_metastore_proto != nil {
		return
	}
	file_engine_proto_error_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_engine_proto_metastore_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RegisterMetaStoreRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_engine_proto_metastore_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RegisterMetaStoreResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_engine_proto_metastore_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryMetaStoreRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_engine_proto_metastore_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryMetaStoreResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_engine_proto_metastore_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_engine_proto_metastore_proto_goTypes,
		DependencyIndexes: file_engine_proto_metastore_proto_depIdxs,
		EnumInfos:         file_engine_proto_metastore_proto_enumTypes,
		MessageInfos:      file_engine_proto_metastore_proto_msgTypes,
	}.Build()
	File_engine_proto_metastore_proto = out.File
	file_engine_proto_metastore_proto_rawDesc = nil
	file_engine_proto_metastore_proto_goTypes = nil
	file_engine_proto_metastore_proto_depIdxs = nil
}
