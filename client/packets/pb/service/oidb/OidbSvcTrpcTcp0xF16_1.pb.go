// Code generated by protoc-gen-golite. DO NOT EDIT.
// source: pb/service/oidb/OidbSvcTrpcTcp0xF16_1.proto

package oidb

// Group Remark
type OidbSvcTrpcTcp0XF16_1 struct {
	Body *OidbSvcTrpcTcp0XF16_1Body `protobuf:"bytes,1,opt"`
	_    [0]func()
}

type OidbSvcTrpcTcp0XF16_1Body struct {
	GroupUin     uint32 `protobuf:"varint,1,opt"`
	TargetRemark string `protobuf:"bytes,3,opt"`
	_            [0]func()
}
