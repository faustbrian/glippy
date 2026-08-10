package hostile

import(z "strings";_ "bytes")

const(Short=0XAB;MuchLongerName=(1+2))

var(x int=1;longName string="x")

type CompatibilityRecord struct{A int;LongField string}

func compatibilityValue()int{return (((0XCAFE)))}

func compatibilityUse()string{return z.TrimSpace("")}
