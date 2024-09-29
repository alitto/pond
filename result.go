package pond

type Result[O any] struct {
	Output O
	Err    error
}
