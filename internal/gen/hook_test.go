package gen_test

import (
	"testing"
)

// hookTypes declares the types the structs of the hook tests hold.
const hookTypes = `package app

import "github.com/go-kanna/validator"

type Address struct {
	Zip string ` + "`json:\"zip\"`" + `
}

type Item struct {
	SKU string ` + "`json:\"sku\"`" + `
}

// Manual validates itself by hand, under names of its own.
type Manual struct{}

func (Manual) Validate() error {
	var errs validator.Errors
	errs.Add(validator.Field("virtual"), validator.Blank())
	return errs.Err()
}

// Wrapper is validated by the Validate of Manual, which it embeds.
type Wrapper struct {
	Manual
}
`

func TestCommand_HookPathsThatCanBe(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Order struct {
	Name    string          ` + "`json:\"name\"`" + `
	Address Address         ` + "`json:\"address\"`" + `
	Items   []Item          ` + "`json:\"items\"`" + `
	Grid    [][]string      ` + "`json:\"grid\"`" + `
	Labels  map[string]Item ` + "`json:\"labels\"`" + `
	Dotted  string          ` + "`json:\"a.b\"`" + `
	Secret  string          ` + "`json:\"-\"`" + `
	Payload any             ` + "`json:\"payload\"`" + `
	Checked Manual          ` + "`json:\"checked\"`" + `
	Wrapped Wrapper         ` + "`json:\"wrapped\"`" + `
	note    string
}

const named = "name"

func (o Order) validate(errs *validator.Errors) {
	errs.Add(validator.Field(named), nil)
	errs.Add(validator.Base, nil)
	errs.Add(validator.Field("address", "zip"), nil)
	errs.Add(validator.Field("address", "base"), nil)
	errs.Add(validator.Field("grid").Index(0), nil)
	errs.Add(validator.Field("a.b"), nil)
	errs.Add(validator.Field("Secret"), nil)
	errs.Add(validator.Field("note"), nil)
	errs.Add(validator.Field("payload", "anything"), nil)
	errs.Add(validator.Field("payload").Index(0), nil)
	errs.Add(validator.Field("checked", "anything"), nil)
	errs.Add(validator.Field("wrapped", "virtual"), nil)
	errs.Add(validator.Field("items").Index(1), nil)
	errs.Add(validator.Field("grid").Index(0).Index(1), nil)
	errs.Add(validator.Field("labels").Key("x"), nil)
	errs.Add((validator.Field("name")), nil)
	errs.Nest(validator.Path{}, nil)
	errs.Nest(validator.Field("address"), nil)
	errs.Nest(validator.Field("items").Index(0), nil)
	errs.Add(validator.Field("items").Index(1).Field("sku"), nil)
	func() {
		errs.Add(validator.Field("address", "zip"), nil)
		errs := &validator.Errors{}
		errs.Add(validator.Field("nothing"), nil)
	}()
	var other validator.Errors
	other.Add(validator.Field("nothing"), nil)
	other.Add(validator.Field("nothing").Index(0), nil)
	names := []string{"nothing"}
	errs.Add(validator.Field(names[0]), nil)
	errs.Add(validator.Field("nothing", names...), nil)
	_ = o.note
}

// Flat reports, through the Validate of Manual, names no field of it tells.
//
//kanna:validate
type Flat struct {
	Manual
}

func (f Flat) validate(errs *validator.Errors) {
	errs.Add(validator.Field("virtual"), nil)
}
`,
	})

	if code, _, stderr := run(t, dir); code != 0 || stderr != "" {
		t.Errorf("exit %d, stderr:\n%s", code, stderr)
	}
}

func TestCommand_HookPathsThatCannotBe(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Order struct {
	Name    string          ` + "`json:\"name\"`" + `
	Age     int             ` + "`json:\"age\"`" + `
	Address Address         ` + "`json:\"address\"`" + `
	Items   []Item          ` + "`json:\"items\"`" + `
	Fixed   [2]Item         ` + "`json:\"fixed\"`" + `
	Labels  map[string]Item ` + "`json:\"labels\"`" + `
	Wrapped Wrapper         ` + "`json:\"wrapped\"`" + `
}

const typo = "nme"

func (o Order) validate(errs *validator.Errors) {
	errs.Add(validator.Field("aeg"), nil)
	errs.Add(validator.Field(typo), nil)
	errs.Add(validator.Field("zzzzzz"), nil)
	errs.Add(validator.Field("address", "zpi"), nil)
	errs.Add(validator.Field("age", "years"), nil)
	errs.Add(validator.Field("items", "sku"), nil)
	errs.Add(validator.Field("fixed", "sku"), nil)
	errs.Add(validator.Field("labels", "x"), nil)
	errs.Add(validator.Field("labels").Index(0), nil)
	errs.Add(validator.Field("address").Index(0), nil)
	errs.Add(validator.Field("base", "zip"), nil)
	errs.Add(validator.Path{}, nil)
	errs.Add(validator.Field("name").Index(0), nil)
	errs.Add(validator.Field("items").Key("x"), nil)
	errs.Nest(validator.Path{}.Index(0), nil)
	errs.Add(validator.Field("virtual"), nil)
	errs.Add(validator.Field("address.zip"), nil)
}
`,
	})

	code, _, stderr := run(t, dir)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	want := `./order.go:19:11: error: "aeg" is not a path of Order: Order has no field "aeg"
  did you mean "age"?
./order.go:20:11: error: "nme" is not a path of Order: Order has no field "nme"
  did you mean "name"?
./order.go:21:11: error: "zzzzzz" is not a path of Order: Order has no field "zzzzzz"
  an error on Order as a whole goes under validator.Base
./order.go:22:11: error: "address.zpi" is not a path of Order: address has no field "zpi"
  did you mean "address.zip"?
./order.go:23:11: error: "age.years" is not a path of Order: age is int, with nothing under it
./order.go:24:11: error: "items.sku" is not a path of Order: items is a slice, whose elements go under Path.Index
./order.go:25:11: error: "fixed.sku" is not a path of Order: fixed is an array, whose elements go under Path.Index
./order.go:26:11: error: "labels.x" is not a path of Order: labels is a map, whose values go under Path.Key
./order.go:27:11: error: validator.Field("labels").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not map[string]Item
./order.go:28:11: error: validator.Field("address").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not Address
./order.go:29:11: error: "base.zip" is not a path of Order: base is the value as a whole, with nothing under it
./order.go:30:11: error: validator.Path{} is not a path of Order: Add needs a field
  an error on Order as a whole goes under validator.Base
./order.go:31:11: error: validator.Field("name").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not string
./order.go:32:11: error: validator.Field("items").Key("x") is not a path of Order: ` +
		`Path.Key needs a map, not []Item
./order.go:33:12: error: validator.Path{}.Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not Order
./order.go:34:11: error: "virtual" is not a path of Order: Order has no field "virtual"
  an error on Order as a whole goes under validator.Base
./order.go:35:11: error: "address.zip" is not a path of Order: Order has no field "address.zip"
  a name is one field: give each to validator.Field, as in validator.Field("address", "zip")
`
	if stderr != want {
		t.Errorf("stderr:\n%s\nwant:\n%s", stderr, want)
	}
}
