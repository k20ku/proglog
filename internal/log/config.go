package log

type Config struct {
	Segment struct {
		MaxStoreBytes uint64 // e.g. 16 << 20 // 16MiB
		MaxIndexBytes uint64 // e.g. 64 << 20 // 64MiB
		InitialOffset uint64 // probably, InitialBaseOffset...
	}
}
