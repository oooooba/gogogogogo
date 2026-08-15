package main

import (
	"context"
	"errors"
)

type myKey struct{ k int }

func main() {
	bad := 0
	chk := func(name string, got, want string) {
		if got != want {
			println("FAIL " + name + ": got=" + got + " want=" + want)
			bad++
		}
	}
	chkb := func(name string, got, want bool) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}

	ctx := context.Background()
	chkb("BackgroundErrNil", ctx.Err() == nil, true)
	chkb("BackgroundDoneNil", ctx.Done() == nil, true)
	_, ok := ctx.Deadline()
	chkb("BackgroundDeadline", ok, false)
	chkb("BackgroundValueNil", ctx.Value("x") == nil, true)

	chkb("TODOErrNil", context.TODO().Err() == nil, true)
	chkb("TODODoneNil", context.TODO().Done() == nil, true)

	ctx2, cancel := context.WithCancel(context.Background())
	chkb("WithCancelErrNil", ctx2.Err() == nil, true)
	chkb("WithCancelDoneNonNil", ctx2.Done() != nil, true)
	_, ok = ctx2.Deadline()
	chkb("WithCancelDeadline", ok, false)
	cancel()
	chkb("WithCancelCanceled", ctx2.Err() == context.Canceled, true)
	<-ctx2.Done()
	chk("DoneClosed", "done-closed", "done-closed")
	cancel()
	chkb("CancelTwice", ctx2.Err() == context.Canceled, true)

	cctx, ccancel := context.WithCancelCause(context.Background())
	ccancel(errors.New("boom"))
	chk("Cause", context.Cause(cctx).Error(), "boom")
	chkb("CauseCanceled", cctx.Err() == context.Canceled, true)

	ctx3, cancel3 := context.WithCancel(context.Background())
	ctx4, cancel4 := context.WithCancel(ctx3)
	cancel3()
	chkb("PropagateCancel", ctx4.Err() == context.Canceled, true)
	cancel4()

	vctx := context.WithValue(context.Background(), myKey{1}, "one")
	chk("WithValue", vctx.Value(myKey{1}).(string), "one")
	chkb("WithValueOther", vctx.Value(myKey{2}) == nil, true)

	child := context.WithValue(vctx, "s", "str")
	chk("NestedValue", child.Value(myKey{1}).(string), "one")
	chk("NestedValue2", child.Value("s").(string), "str")

	cc2, ccancel2 := context.WithCancelCause(vctx)
	_ = ccancel2
	chk("ParentValue", cc2.Value(myKey{1}).(string), "one")

	wctx := context.WithoutCancel(vctx)
	chk("WithoutCancelValue", wctx.Value(myKey{1}).(string), "one")
	chkb("WithoutCancelErrNil", wctx.Err() == nil, true)

	chk("Canceled", context.Canceled.Error(), "context canceled")
	chk("DeadlineExceeded", context.DeadlineExceeded.Error(), "context deadline exceeded")

	println("bad:", bad)
}
