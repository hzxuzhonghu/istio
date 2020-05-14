// Code generated by protoc-gen-go. DO NOT EDIT.
// source: envoy/config/filter/http/rate_limit/v2/rate_limit.proto

package envoy_config_filter_http_rate_limit_v2

import (
	fmt "fmt"
	_ "github.com/cncf/udpa/go/udpa/annotations"
	v2 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v2"
	_ "github.com/envoyproxy/protoc-gen-validate/validate"
	proto "github.com/golang/protobuf/proto"
	duration "github.com/golang/protobuf/ptypes/duration"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type RateLimit struct {
	Domain                         string                     `protobuf:"bytes,1,opt,name=domain,proto3" json:"domain,omitempty"`
	Stage                          uint32                     `protobuf:"varint,2,opt,name=stage,proto3" json:"stage,omitempty"`
	RequestType                    string                     `protobuf:"bytes,3,opt,name=request_type,json=requestType,proto3" json:"request_type,omitempty"`
	Timeout                        *duration.Duration         `protobuf:"bytes,4,opt,name=timeout,proto3" json:"timeout,omitempty"`
	FailureModeDeny                bool                       `protobuf:"varint,5,opt,name=failure_mode_deny,json=failureModeDeny,proto3" json:"failure_mode_deny,omitempty"`
	RateLimitedAsResourceExhausted bool                       `protobuf:"varint,6,opt,name=rate_limited_as_resource_exhausted,json=rateLimitedAsResourceExhausted,proto3" json:"rate_limited_as_resource_exhausted,omitempty"`
	RateLimitService               *v2.RateLimitServiceConfig `protobuf:"bytes,7,opt,name=rate_limit_service,json=rateLimitService,proto3" json:"rate_limit_service,omitempty"`
	XXX_NoUnkeyedLiteral           struct{}                   `json:"-"`
	XXX_unrecognized               []byte                     `json:"-"`
	XXX_sizecache                  int32                      `json:"-"`
}

func (m *RateLimit) Reset()         { *m = RateLimit{} }
func (m *RateLimit) String() string { return proto.CompactTextString(m) }
func (*RateLimit) ProtoMessage()    {}
func (*RateLimit) Descriptor() ([]byte, []int) {
	return fileDescriptor_af348a51c982d3a6, []int{0}
}

func (m *RateLimit) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RateLimit.Unmarshal(m, b)
}
func (m *RateLimit) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RateLimit.Marshal(b, m, deterministic)
}
func (m *RateLimit) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RateLimit.Merge(m, src)
}
func (m *RateLimit) XXX_Size() int {
	return xxx_messageInfo_RateLimit.Size(m)
}
func (m *RateLimit) XXX_DiscardUnknown() {
	xxx_messageInfo_RateLimit.DiscardUnknown(m)
}

var xxx_messageInfo_RateLimit proto.InternalMessageInfo

func (m *RateLimit) GetDomain() string {
	if m != nil {
		return m.Domain
	}
	return ""
}

func (m *RateLimit) GetStage() uint32 {
	if m != nil {
		return m.Stage
	}
	return 0
}

func (m *RateLimit) GetRequestType() string {
	if m != nil {
		return m.RequestType
	}
	return ""
}

func (m *RateLimit) GetTimeout() *duration.Duration {
	if m != nil {
		return m.Timeout
	}
	return nil
}

func (m *RateLimit) GetFailureModeDeny() bool {
	if m != nil {
		return m.FailureModeDeny
	}
	return false
}

func (m *RateLimit) GetRateLimitedAsResourceExhausted() bool {
	if m != nil {
		return m.RateLimitedAsResourceExhausted
	}
	return false
}

func (m *RateLimit) GetRateLimitService() *v2.RateLimitServiceConfig {
	if m != nil {
		return m.RateLimitService
	}
	return nil
}

func init() {
	proto.RegisterType((*RateLimit)(nil), "envoy.config.filter.http.rate_limit.v2.RateLimit")
}

func init() {
	proto.RegisterFile("envoy/config/filter/http/rate_limit/v2/rate_limit.proto", fileDescriptor_af348a51c982d3a6)
}

var fileDescriptor_af348a51c982d3a6 = []byte{
	// 492 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x52, 0xbd, 0x8e, 0xd3, 0x40,
	0x10, 0x66, 0x73, 0xf9, 0xbb, 0x3d, 0x7e, 0x82, 0x1b, 0xcc, 0x89, 0x0b, 0xe1, 0x90, 0x50, 0x14,
	0xa1, 0xb5, 0x48, 0x90, 0xa8, 0x31, 0xa1, 0x41, 0x20, 0x9d, 0x16, 0x7a, 0x6b, 0x13, 0x4f, 0x92,
	0x95, 0xec, 0x5d, 0xb3, 0x3b, 0xb6, 0xe2, 0x0e, 0x51, 0xd2, 0xd0, 0xf2, 0x0a, 0xbc, 0x02, 0x4f,
	0x40, 0xcb, 0xab, 0x50, 0xa6, 0x40, 0xc8, 0x7f, 0xc9, 0x1d, 0x34, 0xd7, 0xed, 0xce, 0xf7, 0x33,
	0xa3, 0x6f, 0x86, 0xbe, 0x00, 0x95, 0xe9, 0xdc, 0x5b, 0x6a, 0xb5, 0x92, 0x6b, 0x6f, 0x25, 0x23,
	0x04, 0xe3, 0x6d, 0x10, 0x13, 0xcf, 0x08, 0x84, 0x20, 0x92, 0xb1, 0x44, 0x2f, 0x9b, 0x5e, 0xfa,
	0xb1, 0xc4, 0x68, 0xd4, 0xce, 0x93, 0x52, 0xc8, 0x2a, 0x21, 0xab, 0x84, 0xac, 0x10, 0xb2, 0x4b,
	0xd4, 0x6c, 0x7a, 0xfa, 0xf8, 0x4a, 0x83, 0x02, 0x3b, 0x78, 0x46, 0xb6, 0x32, 0x3b, 0x1d, 0xae,
	0xb5, 0x5e, 0x47, 0xe0, 0x95, 0xbf, 0x45, 0xba, 0xf2, 0xc2, 0xd4, 0x08, 0x94, 0x5a, 0x35, 0x78,
	0x1a, 0x26, 0xc2, 0x13, 0x4a, 0x69, 0x2c, 0xcb, 0xd6, 0x8b, 0xe5, 0xba, 0xf0, 0xaa, 0xf1, 0xb3,
	0xff, 0x70, 0x8b, 0x02, 0xd3, 0xc6, 0xfe, 0x5e, 0x26, 0x22, 0x19, 0x0a, 0x04, 0xaf, 0x79, 0x54,
	0xc0, 0xf9, 0xf7, 0x23, 0x7a, 0xcc, 0x05, 0xc2, 0xdb, 0x62, 0x24, 0xe7, 0x21, 0xed, 0x86, 0x3a,
	0x16, 0x52, 0xb9, 0x64, 0x44, 0xc6, 0xc7, 0x7e, 0x6f, 0xe7, 0xb7, 0x4d, 0x6b, 0x44, 0x78, 0x5d,
	0x76, 0xce, 0x68, 0xc7, 0xa2, 0x58, 0x83, 0xdb, 0x1a, 0x91, 0xf1, 0xad, 0x12, 0x9f, 0xb4, 0x5c,
	0xca, 0xab, 0xaa, 0x33, 0xa7, 0x37, 0x0d, 0x7c, 0x4c, 0xc1, 0x62, 0x80, 0x79, 0x02, 0xee, 0x51,
	0xe9, 0xf2, 0x68, 0xe7, 0x0f, 0xcd, 0x03, 0xde, 0x97, 0x0a, 0xc1, 0x28, 0x11, 0xf1, 0x3e, 0x6c,
	0xeb, 0x57, 0x7b, 0xa1, 0x71, 0xc3, 0x6f, 0xf0, 0x93, 0x5a, 0xf6, 0x21, 0x4f, 0xc0, 0x99, 0xd1,
	0x1e, 0xca, 0x18, 0x74, 0x8a, 0x6e, 0x7b, 0x44, 0xc6, 0x27, 0xd3, 0xfb, 0xac, 0x4a, 0x87, 0x35,
	0xe9, 0xb0, 0x79, 0x9d, 0x0e, 0x6f, 0x98, 0xce, 0x84, 0xde, 0x5d, 0x09, 0x19, 0xa5, 0x06, 0x82,
	0x58, 0x87, 0x10, 0x84, 0xa0, 0x72, 0xb7, 0x33, 0x22, 0xe3, 0x3e, 0xbf, 0x53, 0x03, 0xef, 0x74,
	0x08, 0x73, 0x50, 0xb9, 0xf3, 0x86, 0x9e, 0x1f, 0x56, 0x04, 0x61, 0x20, 0x6c, 0x60, 0xc0, 0xea,
	0xd4, 0x2c, 0x21, 0x80, 0xed, 0x46, 0xa4, 0x16, 0x21, 0x74, 0xbb, 0xa5, 0x78, 0x68, 0x9a, 0x74,
	0x20, 0x7c, 0x69, 0x79, 0x4d, 0x7b, 0xdd, 0xb0, 0x1c, 0x49, 0x9d, 0x83, 0x57, 0x60, 0xc1, 0x64,
	0x72, 0x09, 0x6e, 0xaf, 0x9c, 0xfb, 0x19, 0xbb, 0x72, 0x22, 0xfb, 0xd5, 0xb3, 0x6c, 0xca, 0xf6,
	0xa1, 0xbf, 0xaf, 0x24, 0xaf, 0x4a, 0x8e, 0xdf, 0xdf, 0xf9, 0x9d, 0x2f, 0xa4, 0x35, 0x20, 0x7c,
	0x60, 0xfe, 0x61, 0xf8, 0x9f, 0xc9, 0xef, 0x6f, 0x7f, 0xbe, 0x76, 0x9e, 0x3a, 0x93, 0xca, 0xb6,
	0x48, 0x51, 0xd9, 0x62, 0xd9, 0xf5, 0xf5, 0xd9, 0xc3, 0xf9, 0xd5, 0x7d, 0x66, 0x3f, 0x3e, 0xfd,
	0xfc, 0xd5, 0x6d, 0x0d, 0x08, 0x7d, 0x2e, 0x75, 0x35, 0x4d, 0x62, 0xf4, 0x36, 0x67, 0xd7, 0xbb,
	0x5d, 0xff, 0xf6, 0x7e, 0xcc, 0x8b, 0x22, 0xfa, 0x0b, 0xb2, 0xe8, 0x96, 0x3b, 0x98, 0xfd, 0x0d,
	0x00, 0x00, 0xff, 0xff, 0xd3, 0xd2, 0xbe, 0xea, 0x37, 0x03, 0x00, 0x00,
}
