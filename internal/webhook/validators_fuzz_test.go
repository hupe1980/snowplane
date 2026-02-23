package webhook

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// fuzzScheme returns a shared scheme for fuzz tests.
func fuzzScheme() *runtime.Scheme {
	return testScheme()
}

// fuzzCreateRequest builds a CREATE admission request from raw JSON bytes.
func fuzzCreateRequest(raw []byte) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
}

// fuzzUpdateRequest builds an UPDATE admission request with raw JSON for both old and new objects.
func fuzzUpdateRequest(oldRaw, newRaw []byte) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}
}

func FuzzDatabaseValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"db1"}}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"not an object"`))
	f.Add([]byte(`{"spec":null}`))
	f.Add([]byte{0xff, 0xfe, 0x00})

	scheme := fuzzScheme()
	v := NewDatabaseValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		// CREATE — must not panic.
		v.Handle(context.Background(), fuzzCreateRequest(data))

		// UPDATE with empty old object — must not panic.
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))

		// UPDATE with fuzzed old object — must not panic.
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzSchemaValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"s1","databaseRef":{"name":"db"}}}`))
	f.Add([]byte(`null`))
	f.Add([]byte{0xff, 0xfe})

	scheme := fuzzScheme()
	v := NewSchemaValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzWarehouseValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"wh1"}}`))
	f.Add([]byte(`null`))
	f.Add([]byte{0x00})

	scheme := fuzzScheme()
	v := NewWarehouseValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzAccountRoleValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"role1"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewAccountRoleValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzDatabaseRoleValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"dbrole1","databaseRef":{"name":"db"}}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewDatabaseRoleValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzUserValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"user1","type":"PERSON"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewUserValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzAccountRoleGrantValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"privilege":"SELECT","on":{"schemaObject":{"objectType":"TABLE","objectName":"MY_TABLE"}},"accountRole":"ANALYST"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewAccountRoleGrantValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzTableValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"t1","databaseRef":{"name":"db"},"schemaRef":{"name":"sch"},"columns":[{"name":"id","type":"NUMBER"}]}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewTableValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzViewValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"v1","databaseRef":{"name":"db"},"schemaRef":{"name":"sch"},"statement":"SELECT 1"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewViewValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzStageValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"name":"stg1","databaseRef":{"name":"db"},"schemaRef":{"name":"sch"},"stageType":"Internal"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewStageValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}

func FuzzProviderConfigValidator(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spec":{"account":"xy12345","user":"admin","authenticationType":"KeyPair"}}`))
	f.Add([]byte(`null`))

	scheme := fuzzScheme()
	v := NewProviderConfigValidator(scheme)

	f.Fuzz(func(_ *testing.T, data []byte) {
		v.Handle(context.Background(), fuzzCreateRequest(data))
		v.Handle(context.Background(), fuzzUpdateRequest([]byte(`{}`), data))
		v.Handle(context.Background(), fuzzUpdateRequest(data, data))
	})
}
