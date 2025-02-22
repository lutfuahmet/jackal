// Copyright 2022 The jackal Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.19.4
// source: proto/model/v1/last.proto

package lastmodel

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

// Last represents a user last activity entity.
type Last struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Username string `protobuf:"bytes,1,opt,name=username,proto3" json:"username,omitempty"`
	Seconds  int64  `protobuf:"varint,2,opt,name=seconds,proto3" json:"seconds,omitempty"`
	Status   string `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
}

func (x *Last) Reset() {
	*x = Last{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_model_v1_last_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Last) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Last) ProtoMessage() {}

func (x *Last) ProtoReflect() protoreflect.Message {
	mi := &file_proto_model_v1_last_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Last.ProtoReflect.Descriptor instead.
func (*Last) Descriptor() ([]byte, []int) {
	return file_proto_model_v1_last_proto_rawDescGZIP(), []int{0}
}

func (x *Last) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *Last) GetSeconds() int64 {
	if x != nil {
		return x.Seconds
	}
	return 0
}

func (x *Last) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

var File_proto_model_v1_last_proto protoreflect.FileDescriptor

var file_proto_model_v1_last_proto_rawDesc = []byte{
	0x0a, 0x19, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2f, 0x76, 0x31,
	0x2f, 0x6c, 0x61, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0d, 0x6d, 0x6f, 0x64,
	0x65, 0x6c, 0x2e, 0x6c, 0x61, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x22, 0x54, 0x0a, 0x04, 0x4c, 0x61,
	0x73, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18,
	0x0a, 0x07, 0x73, 0x65, 0x63, 0x6f, 0x6e, 0x64, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x07, 0x73, 0x65, 0x63, 0x6f, 0x6e, 0x64, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x42, 0x1b, 0x5a, 0x19, 0x70, 0x6b, 0x67, 0x2f, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2f, 0x6c, 0x61,
	0x73, 0x74, 0x2f, 0x3b, 0x6c, 0x61, 0x73, 0x74, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_proto_model_v1_last_proto_rawDescOnce sync.Once
	file_proto_model_v1_last_proto_rawDescData = file_proto_model_v1_last_proto_rawDesc
)

func file_proto_model_v1_last_proto_rawDescGZIP() []byte {
	file_proto_model_v1_last_proto_rawDescOnce.Do(func() {
		file_proto_model_v1_last_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_model_v1_last_proto_rawDescData)
	})
	return file_proto_model_v1_last_proto_rawDescData
}

var file_proto_model_v1_last_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_proto_model_v1_last_proto_goTypes = []interface{}{
	(*Last)(nil), // 0: model.last.v1.Last
}
var file_proto_model_v1_last_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_proto_model_v1_last_proto_init() }
func file_proto_model_v1_last_proto_init() {
	if File_proto_model_v1_last_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_model_v1_last_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Last); i {
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
			RawDescriptor: file_proto_model_v1_last_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_proto_model_v1_last_proto_goTypes,
		DependencyIndexes: file_proto_model_v1_last_proto_depIdxs,
		MessageInfos:      file_proto_model_v1_last_proto_msgTypes,
	}.Build()
	File_proto_model_v1_last_proto = out.File
	file_proto_model_v1_last_proto_rawDesc = nil
	file_proto_model_v1_last_proto_goTypes = nil
	file_proto_model_v1_last_proto_depIdxs = nil
}
