// Package hook declares validate hooks, which take the runtime's
// *validator.Errors and so need a package loaded within the module.
package hook

import "github.com/go-kanna/validator"

//kanna:validate
type Signup struct {
	Email string `json:"email" validate:"required"`
}

func (s Signup) validate(errs *validator.Errors) {
	if s.Email == "admin@example.com" {
		errs.Add("email", validator.Fail("reserved"))
	}
}

// Audit's hook takes a pointer receiver, which the generated method can call
// on its own copy.
//
//kanna:validate
type Audit struct {
	By string `json:"by" validate:"required"`
}

func (a *Audit) validate(errs *validator.Errors) {
	if a.By == "root" {
		errs.Add("by", validator.Fail("reserved"))
	}
}

// Order embeds Audit, whose hook its own Validate calls; the Validate of Order
// must not call it again.
//
//kanna:validate
type Order struct {
	Audit
}
