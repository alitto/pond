package pond

import (
	"context"
	"errors"
	"fmt"
)

var BackgroundPool = NewPool(context.Background(), 1024)

var ErrPanic = errors.New("task panicked")

type VoidFunc interface {
	~func() | ~func(context.Context) | ~func() error | ~func(context.Context) error
}

type OutputFunc[O any] interface {
	~func() O | ~func(context.Context) O | ~func() (O, error) | ~func(context.Context) (O, error)
}

type InputFunc[I any] interface {
	~func(I) | ~func(context.Context, I) | ~func(I) error | ~func(context.Context, I) error
}

type InputOutputFunc[I, O any] interface {
	~func(I) O | ~func(context.Context, I) O | ~func(I) (O, error) | ~func(context.Context, I) (O, error)
}

type NormalFunc[I, O any] func(I, context.Context) (O, error)

type Task[I, O any] interface {
	Pool() Pool
	Context() context.Context
	Input() I
	WithPool(pool Pool) Task[I, O]
	WithContext(ctx context.Context) Task[I, O]
	WithInput(input I) Task[I, O]
	WithInputs(input ...I) GroupTask[I, O]
	WithMaxConcurrency(maxConcurrency int) Task[I, O]
	Run() TaskContext[O]
}

type task[I, O any] struct {
	pool       Pool
	ctx        context.Context
	input      I
	normalFunc NormalFunc[I, O]
}

func (t *task[I, O]) clone() *task[I, O] {
	return &task[I, O]{
		pool:       t.pool,
		ctx:        t.ctx,
		input:      t.input,
		normalFunc: t.normalFunc,
	}
}

func (t *task[I, O]) Input() I {
	return t.input
}

func (t *task[I, O]) WithInput(input I) Task[I, O] {
	task := t.clone()
	task.input = input
	return task
}

func (t *task[I, O]) WithInputs(inputs ...I) GroupTask[I, O] {
	if inputs == nil {
		panic("inputs cannot be nil")
	}
	return &groupTask[I, O]{
		task:   *t.clone(),
		inputs: inputs,
	}
}

func (t *task[I, O]) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.Pool().Context()
}

func (t *task[I, O]) WithContext(ctx context.Context) Task[I, O] {
	if ctx == nil {
		panic("context cannot be nil")
	}
	task := t.clone()
	task.ctx = ctx
	return task
}

func (t *task[I, O]) Pool() Pool {
	if t.pool != nil {
		return t.pool
	}
	return BackgroundPool
}

func (t *task[I, O]) WithPool(pool Pool) Task[I, O] {
	if pool == nil {
		panic("pool cannot be nil")
	}
	task := t.clone()
	task.pool = pool
	return task
}

func (t *task[I, O]) WithMaxConcurrency(maxConcurrency int) Task[I, O] {
	if maxConcurrency < 0 {
		panic("maxConcurrency must be greater than 0")
	}
	task := t.clone()
	task.pool = t.Pool().Subpool(maxConcurrency)
	return task
}

func (t *task[I, O]) Run() TaskContext[O] {
	childCtx, cancel := context.WithCancelCause(t.Context())

	taskCtx := &taskContext[O]{
		Context: childCtx,
	}

	t.Pool().Submit(func() {
		// Execute function
		output, err := t.normalFunc(t.input, childCtx)

		// Cancel the context to signal task completion
		cancel(&outputOrErr[O]{
			output: output,
			err:    err,
		})
	})

	return taskCtx
}

func NewTask[T VoidFunc](fn T) Task[any, any] {
	return &task[any, any]{
		normalFunc: wrapVoidFunc(fn),
	}
}

func NewITask[I any, T InputFunc[I]](fn T) Task[I, any] {
	return &task[I, any]{
		normalFunc: wrapInputFunc[I](fn),
	}
}

func NewOTask[O any, T OutputFunc[O]](fn T) Task[any, O] {
	return &task[any, O]{
		normalFunc: wrapOutputFunc[O](fn),
	}
}

func NewIOTask[I, O any, T InputOutputFunc[I, O]](fn T) Task[I, O] {
	return &task[I, O]{
		normalFunc: wrapInputOutputFunc[I, O](fn),
	}
}

func wrapVoidFunc[T VoidFunc](task T) NormalFunc[any, any] {

	switch t := any(task).(type) {
	case func():
		return func(_ any, _ context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			t()

			return
		}
	case func(context.Context):
		return func(_ any, ctx context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			t(ctx)

			return
		}
	case func() error:
		return func(_ any, _ context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			err = t()

			return
		}
	case func(context.Context) error:
		return func(_ any, ctx context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			err = t(ctx)

			return
		}
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
}

func wrapInputFunc[I any, T InputFunc[I]](task T) NormalFunc[I, any] {

	switch t := any(task).(type) {
	case func(I):
		return func(input I, _ context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			t(input)

			return
		}
	case func(context.Context, I):
		return func(input I, ctx context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			t(ctx, input)

			return
		}
	case func(I) error:
		return func(input I, _ context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			err = t(input)

			return
		}
	case func(context.Context, I) error:
		return func(input I, ctx context.Context) (output any, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			err = t(ctx, input)

			return
		}
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
}

func wrapOutputFunc[O any, T OutputFunc[O]](task T) NormalFunc[any, O] {

	switch t := any(task).(type) {
	case func() O:
		return func(_ any, _ context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output = t()

			return
		}
	case func(context.Context) O:
		return func(_ any, ctx context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output = t(ctx)

			return
		}
	case func() (O, error):
		return func(_ any, _ context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output, err = t()

			return
		}
	case func(context.Context) (O, error):
		return func(_ any, ctx context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output, err = t(ctx)

			return
		}
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
}

func wrapInputOutputFunc[I, O any, T InputOutputFunc[I, O]](task T) NormalFunc[I, O] {

	switch t := any(task).(type) {
	case func(I) O:
		return func(input I, _ context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output = t(input)

			return
		}
	case func(context.Context, I) O:
		return func(input I, ctx context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output = t(ctx, input)

			return
		}
	case func(I) (O, error):
		return func(input I, _ context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output, err = t(input)

			return
		}
	case func(context.Context, I) (O, error):
		return func(input I, ctx context.Context) (output O, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("%w: %v", ErrPanic, p)
					return
				}
			}()

			output, err = t(ctx, input)

			return
		}
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
}
