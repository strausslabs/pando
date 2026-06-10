package resource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

func errorsAs(err error, target any) bool { return errors.As(err, target) }

var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Kind-conditional spec presence cannot be expressed with field tags alone,
	// so it is enforced at the struct level.
	v.RegisterStructValidation(resourceStructValidation, Resource{})
	return v
}

func resourceStructValidation(sl validator.StructLevel) {
	r := sl.Current().Interface().(Resource)
	switch r.Kind {
	case KindCompose:
		if r.Compose == nil && r.Build == nil {
			sl.ReportError(r.Compose, "compose", "Compose", "compose_or_build", "")
		}
	case KindLocal:
		if r.Local == nil {
			sl.ReportError(r.Local, "local", "Local", "required_for_kind", "")
		}
	case KindTask:
		if r.Task == nil {
			sl.ReportError(r.Task, "task", "Task", "required_for_kind", "")
		}
	}
}

func (r *Resource) Validate() error {
	if err := validate.Struct(r); err != nil {
		return friendly(err)
	}
	return nil
}

func (s *Stack) Validate() error { return s.ValidateExternal(nil) }

// ValidateExternal validates the stack, treating names in `external` as
// satisfiable dependencies that live outside this stack (shared resources).
func (s *Stack) ValidateExternal(external map[string]bool) error {
	if err := validate.Struct(s); err != nil {
		return friendly(err)
	}
	// Relational checks need the whole resource set, so they stay hand-rolled.
	seen := make(map[string]bool, len(s.Resources))
	for _, r := range s.Resources {
		if seen[r.Name] {
			return fmt.Errorf("duplicate resource name %q", r.Name)
		}
		seen[r.Name] = true
	}
	for _, r := range s.Resources {
		for _, d := range r.allDeps() {
			if !seen[d] && !external[d] {
				return fmt.Errorf("resource %q depends on unknown resource %q", r.Name, d)
			}
		}
	}
	return nil
}

func friendly(err error) error {
	var verrs validator.ValidationErrors
	if !errorsAs(err, &verrs) {
		return err
	}
	msgs := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		msgs = append(msgs, describeFieldError(fe))
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}

func describeFieldError(fe validator.FieldError) string {
	field := fe.Namespace()
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "required_if":
		return fmt.Sprintf("%s is required when %s", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of [%s], got %q", field, fe.Param(), fe.Value())
	case "hostname_rfc1123":
		return fmt.Sprintf("%s %q must be a valid dns-style name (lowercase letters, digits, hyphens)", field, fe.Value())
	case "compose_or_build":
		return fmt.Sprintf("%s: compose kind needs a compose or build spec", field)
	case "required_for_kind":
		return fmt.Sprintf("%s spec is required for this kind", field)
	default:
		return fmt.Sprintf("%s failed %q validation", field, fe.Tag())
	}
}
