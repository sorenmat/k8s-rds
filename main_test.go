package main

import (
	"testing"

	"github.com/sorenmat/k8s-rds/crd"
)

func TestConvert(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			Size:     10,
			Username: "smo",
			DBName:   "mydb",
			Engine:   "postgres",
		},
	}
	input := convertSpecToInput(db, "subnet")
	t.Log(input)
	if *input.DBInstanceIdentifier != "mydb" {
		t.Error()
	}
	if *input.Engine != "postgres" {
		t.Error()
	}

}
