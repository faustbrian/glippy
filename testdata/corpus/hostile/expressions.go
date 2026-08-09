package hostile

func conditions(foo, bar, baz, somethingReallyLong bool) bool { if foo && bar && baz && somethingReallyLong { return true };return false }

func call(client interface{ Execute(...any) (any, error) }, values ...any) (any, error) { return client.Execute(values[0],values[1],values[2],values[3],values[4],values[5],values[6]) }
