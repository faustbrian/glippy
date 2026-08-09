package hostile

import "context"

func work(context.Context) int { return 1 }

func statements(ctx context.Context) int { ctx,cancel:=context.WithCancel(ctx);cancel();result:=work(ctx);return result }
