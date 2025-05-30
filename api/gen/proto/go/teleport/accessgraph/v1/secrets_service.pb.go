// Copyright 2024 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        (unknown)
// source: teleport/access_graph/v1/secrets_service.proto

package accessgraphv1

import (
	v1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/devicetrust/v1"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// OperationType is an enum that indicates the operation that the client wants to perform.
type OperationType int32

const (
	// OPERATION_TYPE_UNSPECIFIED is an unknown operation.
	OperationType_OPERATION_TYPE_UNSPECIFIED OperationType = 0
	// OPERATION_TYPE_ADD is an operation that indicates that the client wants to add keys to the list.
	OperationType_OPERATION_TYPE_ADD OperationType = 1
	// OPERATION_TYPE_SYNC is an operation that indicates that the client has sent all the keys and
	// the server can proceed with the analysis.
	OperationType_OPERATION_TYPE_SYNC OperationType = 2
)

// Enum value maps for OperationType.
var (
	OperationType_name = map[int32]string{
		0: "OPERATION_TYPE_UNSPECIFIED",
		1: "OPERATION_TYPE_ADD",
		2: "OPERATION_TYPE_SYNC",
	}
	OperationType_value = map[string]int32{
		"OPERATION_TYPE_UNSPECIFIED": 0,
		"OPERATION_TYPE_ADD":         1,
		"OPERATION_TYPE_SYNC":        2,
	}
)

func (x OperationType) Enum() *OperationType {
	p := new(OperationType)
	*p = x
	return p
}

func (x OperationType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (OperationType) Descriptor() protoreflect.EnumDescriptor {
	return file_teleport_access_graph_v1_secrets_service_proto_enumTypes[0].Descriptor()
}

func (OperationType) Type() protoreflect.EnumType {
	return &file_teleport_access_graph_v1_secrets_service_proto_enumTypes[0]
}

func (x OperationType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use OperationType.Descriptor instead.
func (OperationType) EnumDescriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{0}
}

// ReportAuthorizedKeysRequest is used by Teleport nodes to report authorized keys
// that could be used to bypass Teleport.
type ReportAuthorizedKeysRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// keys is a list of authorized keys that could be used to bypass Teleport.
	Keys []*AuthorizedKey `protobuf:"bytes,1,rep,name=keys,proto3" json:"keys,omitempty"`
	// operation indicates the operation that the client wants to perform.
	Operation     OperationType `protobuf:"varint,2,opt,name=operation,proto3,enum=teleport.access_graph.v1.OperationType" json:"operation,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReportAuthorizedKeysRequest) Reset() {
	*x = ReportAuthorizedKeysRequest{}
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReportAuthorizedKeysRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportAuthorizedKeysRequest) ProtoMessage() {}

func (x *ReportAuthorizedKeysRequest) ProtoReflect() protoreflect.Message {
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportAuthorizedKeysRequest.ProtoReflect.Descriptor instead.
func (*ReportAuthorizedKeysRequest) Descriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{0}
}

func (x *ReportAuthorizedKeysRequest) GetKeys() []*AuthorizedKey {
	if x != nil {
		return x.Keys
	}
	return nil
}

func (x *ReportAuthorizedKeysRequest) GetOperation() OperationType {
	if x != nil {
		return x.Operation
	}
	return OperationType_OPERATION_TYPE_UNSPECIFIED
}

// ReportAuthorizedKeysResponse is the response from ReportAuthorizedKeys
// RPC method.
type ReportAuthorizedKeysResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReportAuthorizedKeysResponse) Reset() {
	*x = ReportAuthorizedKeysResponse{}
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReportAuthorizedKeysResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportAuthorizedKeysResponse) ProtoMessage() {}

func (x *ReportAuthorizedKeysResponse) ProtoReflect() protoreflect.Message {
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportAuthorizedKeysResponse.ProtoReflect.Descriptor instead.
func (*ReportAuthorizedKeysResponse) Descriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{1}
}

// ReportSecretsRequest is used by trusted devices to report secrets found on the host
// that could be used to bypass Teleport.
type ReportSecretsRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Payload:
	//
	//	*ReportSecretsRequest_DeviceAssertion
	//	*ReportSecretsRequest_PrivateKeys
	Payload       isReportSecretsRequest_Payload `protobuf_oneof:"payload"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReportSecretsRequest) Reset() {
	*x = ReportSecretsRequest{}
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReportSecretsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportSecretsRequest) ProtoMessage() {}

func (x *ReportSecretsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportSecretsRequest.ProtoReflect.Descriptor instead.
func (*ReportSecretsRequest) Descriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{2}
}

func (x *ReportSecretsRequest) GetPayload() isReportSecretsRequest_Payload {
	if x != nil {
		return x.Payload
	}
	return nil
}

func (x *ReportSecretsRequest) GetDeviceAssertion() *v1.AssertDeviceRequest {
	if x != nil {
		if x, ok := x.Payload.(*ReportSecretsRequest_DeviceAssertion); ok {
			return x.DeviceAssertion
		}
	}
	return nil
}

func (x *ReportSecretsRequest) GetPrivateKeys() *ReportPrivateKeys {
	if x != nil {
		if x, ok := x.Payload.(*ReportSecretsRequest_PrivateKeys); ok {
			return x.PrivateKeys
		}
	}
	return nil
}

type isReportSecretsRequest_Payload interface {
	isReportSecretsRequest_Payload()
}

type ReportSecretsRequest_DeviceAssertion struct {
	// The device should initiate the device assertion ceremony by sending the
	// AssertDeviceRequest. Please refer to the [teleport.devicetrust.v1.AssertDeviceRequest]
	// message for more details.
	DeviceAssertion *v1.AssertDeviceRequest `protobuf:"bytes,1,opt,name=device_assertion,json=deviceAssertion,proto3,oneof"`
}

type ReportSecretsRequest_PrivateKeys struct {
	// private_keys is a list of private keys that were found on the device.
	PrivateKeys *ReportPrivateKeys `protobuf:"bytes,4,opt,name=private_keys,json=privateKeys,proto3,oneof"`
}

func (*ReportSecretsRequest_DeviceAssertion) isReportSecretsRequest_Payload() {}

func (*ReportSecretsRequest_PrivateKeys) isReportSecretsRequest_Payload() {}

// ReportPrivateKeys is used by trusted devices to report private keys found on the host
// that could be used to bypass Teleport.
type ReportPrivateKeys struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// keys is a list of private keys that could be used to bypass Teleport.
	Keys          []*PrivateKey `protobuf:"bytes,1,rep,name=keys,proto3" json:"keys,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReportPrivateKeys) Reset() {
	*x = ReportPrivateKeys{}
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReportPrivateKeys) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportPrivateKeys) ProtoMessage() {}

func (x *ReportPrivateKeys) ProtoReflect() protoreflect.Message {
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportPrivateKeys.ProtoReflect.Descriptor instead.
func (*ReportPrivateKeys) Descriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{3}
}

func (x *ReportPrivateKeys) GetKeys() []*PrivateKey {
	if x != nil {
		return x.Keys
	}
	return nil
}

// ReportSecretsResponse is the response from the ReportSecrets
// RPC method.
type ReportSecretsResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Payload:
	//
	//	*ReportSecretsResponse_DeviceAssertion
	Payload       isReportSecretsResponse_Payload `protobuf_oneof:"payload"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReportSecretsResponse) Reset() {
	*x = ReportSecretsResponse{}
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReportSecretsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportSecretsResponse) ProtoMessage() {}

func (x *ReportSecretsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_teleport_access_graph_v1_secrets_service_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportSecretsResponse.ProtoReflect.Descriptor instead.
func (*ReportSecretsResponse) Descriptor() ([]byte, []int) {
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP(), []int{4}
}

func (x *ReportSecretsResponse) GetPayload() isReportSecretsResponse_Payload {
	if x != nil {
		return x.Payload
	}
	return nil
}

func (x *ReportSecretsResponse) GetDeviceAssertion() *v1.AssertDeviceResponse {
	if x != nil {
		if x, ok := x.Payload.(*ReportSecretsResponse_DeviceAssertion); ok {
			return x.DeviceAssertion
		}
	}
	return nil
}

type isReportSecretsResponse_Payload interface {
	isReportSecretsResponse_Payload()
}

type ReportSecretsResponse_DeviceAssertion struct {
	// device_assertion is the response from the device assertion ceremony.
	// Please refer to the [teleport.devicetrust.v1.AssertDeviceResponse]
	// message for more details
	DeviceAssertion *v1.AssertDeviceResponse `protobuf:"bytes,1,opt,name=device_assertion,json=deviceAssertion,proto3,oneof"`
}

func (*ReportSecretsResponse_DeviceAssertion) isReportSecretsResponse_Payload() {}

var File_teleport_access_graph_v1_secrets_service_proto protoreflect.FileDescriptor

const file_teleport_access_graph_v1_secrets_service_proto_rawDesc = "" +
	"\n" +
	".teleport/access_graph/v1/secrets_service.proto\x12\x18teleport.access_graph.v1\x1a-teleport/access_graph/v1/authorized_key.proto\x1a*teleport/access_graph/v1/private_key.proto\x1a$teleport/devicetrust/v1/assert.proto\"\xa1\x01\n" +
	"\x1bReportAuthorizedKeysRequest\x12;\n" +
	"\x04keys\x18\x01 \x03(\v2'.teleport.access_graph.v1.AuthorizedKeyR\x04keys\x12E\n" +
	"\toperation\x18\x02 \x01(\x0e2'.teleport.access_graph.v1.OperationTypeR\toperation\"\x1e\n" +
	"\x1cReportAuthorizedKeysResponse\"\xce\x01\n" +
	"\x14ReportSecretsRequest\x12Y\n" +
	"\x10device_assertion\x18\x01 \x01(\v2,.teleport.devicetrust.v1.AssertDeviceRequestH\x00R\x0fdeviceAssertion\x12P\n" +
	"\fprivate_keys\x18\x04 \x01(\v2+.teleport.access_graph.v1.ReportPrivateKeysH\x00R\vprivateKeysB\t\n" +
	"\apayload\"M\n" +
	"\x11ReportPrivateKeys\x128\n" +
	"\x04keys\x18\x01 \x03(\v2$.teleport.access_graph.v1.PrivateKeyR\x04keys\"~\n" +
	"\x15ReportSecretsResponse\x12Z\n" +
	"\x10device_assertion\x18\x01 \x01(\v2-.teleport.devicetrust.v1.AssertDeviceResponseH\x00R\x0fdeviceAssertionB\t\n" +
	"\apayload*`\n" +
	"\rOperationType\x12\x1e\n" +
	"\x1aOPERATION_TYPE_UNSPECIFIED\x10\x00\x12\x16\n" +
	"\x12OPERATION_TYPE_ADD\x10\x01\x12\x17\n" +
	"\x13OPERATION_TYPE_SYNC\x10\x022\x9d\x02\n" +
	"\x15SecretsScannerService\x12\x8b\x01\n" +
	"\x14ReportAuthorizedKeys\x125.teleport.access_graph.v1.ReportAuthorizedKeysRequest\x1a6.teleport.access_graph.v1.ReportAuthorizedKeysResponse\"\x00(\x010\x01\x12v\n" +
	"\rReportSecrets\x12..teleport.access_graph.v1.ReportSecretsRequest\x1a/.teleport.access_graph.v1.ReportSecretsResponse\"\x00(\x010\x01BZZXgithub.com/gravitational/teleport/api/gen/proto/go/teleport/accessgraph/v1;accessgraphv1b\x06proto3"

var (
	file_teleport_access_graph_v1_secrets_service_proto_rawDescOnce sync.Once
	file_teleport_access_graph_v1_secrets_service_proto_rawDescData []byte
)

func file_teleport_access_graph_v1_secrets_service_proto_rawDescGZIP() []byte {
	file_teleport_access_graph_v1_secrets_service_proto_rawDescOnce.Do(func() {
		file_teleport_access_graph_v1_secrets_service_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_teleport_access_graph_v1_secrets_service_proto_rawDesc), len(file_teleport_access_graph_v1_secrets_service_proto_rawDesc)))
	})
	return file_teleport_access_graph_v1_secrets_service_proto_rawDescData
}

var file_teleport_access_graph_v1_secrets_service_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_teleport_access_graph_v1_secrets_service_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_teleport_access_graph_v1_secrets_service_proto_goTypes = []any{
	(OperationType)(0),                   // 0: teleport.access_graph.v1.OperationType
	(*ReportAuthorizedKeysRequest)(nil),  // 1: teleport.access_graph.v1.ReportAuthorizedKeysRequest
	(*ReportAuthorizedKeysResponse)(nil), // 2: teleport.access_graph.v1.ReportAuthorizedKeysResponse
	(*ReportSecretsRequest)(nil),         // 3: teleport.access_graph.v1.ReportSecretsRequest
	(*ReportPrivateKeys)(nil),            // 4: teleport.access_graph.v1.ReportPrivateKeys
	(*ReportSecretsResponse)(nil),        // 5: teleport.access_graph.v1.ReportSecretsResponse
	(*AuthorizedKey)(nil),                // 6: teleport.access_graph.v1.AuthorizedKey
	(*v1.AssertDeviceRequest)(nil),       // 7: teleport.devicetrust.v1.AssertDeviceRequest
	(*PrivateKey)(nil),                   // 8: teleport.access_graph.v1.PrivateKey
	(*v1.AssertDeviceResponse)(nil),      // 9: teleport.devicetrust.v1.AssertDeviceResponse
}
var file_teleport_access_graph_v1_secrets_service_proto_depIdxs = []int32{
	6, // 0: teleport.access_graph.v1.ReportAuthorizedKeysRequest.keys:type_name -> teleport.access_graph.v1.AuthorizedKey
	0, // 1: teleport.access_graph.v1.ReportAuthorizedKeysRequest.operation:type_name -> teleport.access_graph.v1.OperationType
	7, // 2: teleport.access_graph.v1.ReportSecretsRequest.device_assertion:type_name -> teleport.devicetrust.v1.AssertDeviceRequest
	4, // 3: teleport.access_graph.v1.ReportSecretsRequest.private_keys:type_name -> teleport.access_graph.v1.ReportPrivateKeys
	8, // 4: teleport.access_graph.v1.ReportPrivateKeys.keys:type_name -> teleport.access_graph.v1.PrivateKey
	9, // 5: teleport.access_graph.v1.ReportSecretsResponse.device_assertion:type_name -> teleport.devicetrust.v1.AssertDeviceResponse
	1, // 6: teleport.access_graph.v1.SecretsScannerService.ReportAuthorizedKeys:input_type -> teleport.access_graph.v1.ReportAuthorizedKeysRequest
	3, // 7: teleport.access_graph.v1.SecretsScannerService.ReportSecrets:input_type -> teleport.access_graph.v1.ReportSecretsRequest
	2, // 8: teleport.access_graph.v1.SecretsScannerService.ReportAuthorizedKeys:output_type -> teleport.access_graph.v1.ReportAuthorizedKeysResponse
	5, // 9: teleport.access_graph.v1.SecretsScannerService.ReportSecrets:output_type -> teleport.access_graph.v1.ReportSecretsResponse
	8, // [8:10] is the sub-list for method output_type
	6, // [6:8] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	6, // [6:6] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_teleport_access_graph_v1_secrets_service_proto_init() }
func file_teleport_access_graph_v1_secrets_service_proto_init() {
	if File_teleport_access_graph_v1_secrets_service_proto != nil {
		return
	}
	file_teleport_access_graph_v1_authorized_key_proto_init()
	file_teleport_access_graph_v1_private_key_proto_init()
	file_teleport_access_graph_v1_secrets_service_proto_msgTypes[2].OneofWrappers = []any{
		(*ReportSecretsRequest_DeviceAssertion)(nil),
		(*ReportSecretsRequest_PrivateKeys)(nil),
	}
	file_teleport_access_graph_v1_secrets_service_proto_msgTypes[4].OneofWrappers = []any{
		(*ReportSecretsResponse_DeviceAssertion)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_teleport_access_graph_v1_secrets_service_proto_rawDesc), len(file_teleport_access_graph_v1_secrets_service_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_teleport_access_graph_v1_secrets_service_proto_goTypes,
		DependencyIndexes: file_teleport_access_graph_v1_secrets_service_proto_depIdxs,
		EnumInfos:         file_teleport_access_graph_v1_secrets_service_proto_enumTypes,
		MessageInfos:      file_teleport_access_graph_v1_secrets_service_proto_msgTypes,
	}.Build()
	File_teleport_access_graph_v1_secrets_service_proto = out.File
	file_teleport_access_graph_v1_secrets_service_proto_goTypes = nil
	file_teleport_access_graph_v1_secrets_service_proto_depIdxs = nil
}
