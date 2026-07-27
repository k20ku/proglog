package log

type Config struct {
	Segment SegmentConfig
}

type SegmentConfig struct {
	MaxStoreBytes uint64 // e.g. 16 << 20 // 16MiB
	MaxIndexBytes uint64 // e.g. 64 << 20 // 64MiB
	InitialOffset uint64 // probably, InitialBaseOffset...
}

func NewSegemntConfig() SegmentConfig {
	return SegmentConfig{
		MaxStoreBytes: 16 << 20, // 16 MiB
		MaxIndexBytes: 1 << 20,  // 1 Mib
		InitialOffset: 0,
	}
}

func NewConfig() Config {
	return Config{
		Segment: NewSegemntConfig(),
	}
}
