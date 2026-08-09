package hostile

type Number interface{ ~int | ~int64 | ~float64 }

func sum[T Number](values []T) T { var result T;for _,value:=range values{result+=value};return result }
