package geerpc

import "gee-rpc/codec"

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber uint32
	CodecType   codec.Type
}

type SetOption func(opt *Option)

func SetMagicNumber(magic uint32) SetOption {
	return func(opt *Option) {
		opt.MagicNumber = magic
	}
}

func SetCodecType(codecType codec.Type) SetOption {
	return func(opt *Option) {
		opt.CodecType = codecType
	}
}

func NewOption(opts ...SetOption) *Option {
	opt := &Option{
		MagicNumber: MagicNumber,
		CodecType:   codec.GobType,
	}
	for _, set := range opts {
		set(opt)
	}
	return opt
}

var DefaultOption = NewOption()
