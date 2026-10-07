package ingest

import (
 "os"
 "strings"
 "testing"
)

func TestNashSampleALTO(t *testing.T){
 b,err:=os.ReadFile("../../../../data/starter-corpus/qa/nash-page-0049.alto.xml");if err!=nil{t.Fatal(err)}
 text,err:=ParseALTO(b);if err!=nil{t.Fatal(err)}
 if !strings.Contains(text,"CARBO VEGETABILIS")||len(text)<1000{t.Fatalf("unexpected OCR text: %.100s",text)}
}
