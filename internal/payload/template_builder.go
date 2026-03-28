package payload

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// GeneratePayloadTemplate builds a JSON payload template for a given RPC input message descriptor.
// The template is intended to be processed by Generator.ReplaceFakerTokens(). Because tokens are
// embedded, intermediate JSON may not be valid until token replacement happens.
func GeneratePayloadTemplate(inputType *desc.MessageDescriptor, maxDepth int) (string, []string, error) {
	if inputType == nil {
		return "{}", nil, nil
	}
	if maxDepth <= 0 {
		maxDepth = 5
	}

	b := &templateBuilder{
		maxDepth:    maxDepth,
		maxNodes:    5000,
		safeToken:   "{{faker.safeAlphaNum}}",
		warnings:    nil,
		unknownType: nil,
	}

	out := b.buildMessage(inputType, 0)
	if out == "" {
		out = "{}"
	}
	return out, b.warnings, nil
}

type templateBuilder struct {
	maxDepth  int
	maxNodes  int
	safeToken string

	nodes int

	warnings    []string
	unknownType map[string]struct{}
}

func (b *templateBuilder) incNode() bool {
	b.nodes++
	return b.nodes <= b.maxNodes
}

func (b *templateBuilder) buildMessage(md *desc.MessageDescriptor, depth int) string {
	if md == nil || depth > b.maxDepth {
		return "{}"
	}
	if !b.incNode() {
		return "{}"
	}

	// Oneof: choose only the first choice per oneof set to avoid setting multiple fields at once.
	chosenOneof := make(map[int32]struct{}, 0)
	for _, od := range md.GetOneOfs() {
		choices := od.GetChoices()
		if len(choices) > 0 {
			chosenOneof[choices[0].GetNumber()] = struct{}{}
		}
	}

	var sb strings.Builder
	sb.WriteByte('{')

	first := true
	for _, fd := range md.GetFields() {
		if fd == nil {
			continue
		}
		if fd.GetOneOf() != nil {
			if _, ok := chosenOneof[fd.GetNumber()]; !ok {
				continue
			}
		}

		val, ok := b.buildField(fd, depth)
		if !ok {
			continue
		}

		if !first {
			sb.WriteByte(',')
		}
		first = false

		sb.WriteString(strconv.Quote(fd.GetName()))
		sb.WriteByte(':')
		sb.WriteString(val)
	}

	sb.WriteByte('}')
	return sb.String()
}

func (b *templateBuilder) buildField(fd *desc.FieldDescriptor, depth int) (string, bool) {
	if fd == nil {
		return "null", false
	}

	// Map fields are represented as JSON objects.
	if fd.IsMap() {
		if !b.incNode() {
			return "{}", true
		}
		return "{}", true
	}

	// Repeated fields are represented as JSON arrays.
	if fd.IsRepeated() {
		return b.buildRepeatedField(fd, depth)
	}

	return b.buildScalarOrMessageField(fd, depth)
}

func (b *templateBuilder) buildRepeatedField(fd *desc.FieldDescriptor, depth int) (string, bool) {
	// Spec requirement: repeated arrays are fixed length 2.
	const repeatedLen = 2

	innerFD := fd
	innerKind := fd.GetType()

	_ = innerKind // keep explicit for readability

	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < repeatedLen; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		v, ok := b.buildScalarOrMessageField(innerFD, depth)
		if !ok {
			v = "null"
		}
		sb.WriteString(v)
	}
	sb.WriteByte(']')
	return sb.String(), true
}

func (b *templateBuilder) buildScalarOrMessageField(fd *desc.FieldDescriptor, depth int) (string, bool) {
	switch fd.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		token := b.guessStringToken(fd.GetName())
		return strconv.Quote(token), true
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "{{faker.bool}}", true
	case descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		// Single int token works for int32/int64/sint*/fixed*/uint* in our current generator.
		return "{{faker.int}}", true
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		// No float token in current generator; safe fallback.
		b.warnings = append(b.warnings, fmt.Sprintf("float fallback for field %q", fd.GetName()))
		return "0", true
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		enumType := fd.GetEnumType()
		if enumType == nil {
			return "\"0\"", true
		}
		vals := enumType.GetValues()
		if len(vals) == 0 {
			return "\"0\"", true
		}
		return strconv.Quote(b.chooseEnumValueName(vals)), true
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		// Protobuf bytes are base64 strings in JSON; empty bytes => empty base64 string.
		return `""`, true
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		msgType := fd.GetMessageType()
		return b.buildMessageOrWellKnown(msgType, depth+1), true
	default:
		// Unknown scalar type: safest is to return empty string for strings-like fields.
		// This should be rare.
		return strconv.Quote(b.safeToken), true
	}
}

func (b *templateBuilder) chooseEnumValueName(vals []*desc.EnumValueDescriptor) string {
	// В некоторых протосхемах первым значением идёт *_INVALID (или аналогичный "заглушечный" enum),
	// который нельзя использовать в запросах по умолчанию.
	for _, v := range vals {
		if v == nil {
			continue
		}
		name := v.GetName()
		if name == "" {
			continue
		}
		if strings.HasSuffix(name, "_INVALID") || strings.HasSuffix(name, "_UNKNOWN") {
			continue
		}
		return name
	}

	// Фоллбек: возвращаем первое значение как раньше, чтобы не ломать генерацию на редких схемах,
	// где все значения отмечены как INVALID (или список заполнен странно).
	if len(vals) > 0 && vals[0] != nil && vals[0].GetName() != "" {
		b.warnings = append(b.warnings, "enum has only *_INVALID values; falling back to first")
		return vals[0].GetName()
	}
	return "0"
}

func (b *templateBuilder) buildMessageOrWellKnown(md *desc.MessageDescriptor, depth int) string {
	if md == nil || depth > b.maxDepth {
		return "{}"
	}

	fqn := md.GetFullyQualifiedName()
	switch fqn {
	case "google.protobuf.Timestamp":
		return strconv.Quote("{{faker.timestamp}}")
	case "google.protobuf.Duration":
		return strconv.Quote("0s")
	case "google.protobuf.Empty":
		return "{}"
	case "google.protobuf.Any":
		// jsonpb expects: {"@type":"type.googleapis.com/<pkg.Msg>", "value":"<base64>"}
		// Empty message bytes => empty base64 string.
		return `{"@type":"type.googleapis.com/google.protobuf.Empty","value":""}`
	case "google.protobuf.Struct":
		return "{}"
	case "google.protobuf.Value":
		return `{"stringValue":` + strconv.Quote(b.safeToken) + `}`
	case "google.protobuf.ListValue":
		return `{"values":[{"stringValue":` + strconv.Quote(b.safeToken) + `},{"stringValue":` + strconv.Quote(b.safeToken) + `}]}`
	case "google.protobuf.StringValue":
		return strconv.Quote(b.safeToken)
	case "google.protobuf.BytesValue":
		return `""`
	case "google.protobuf.BoolValue":
		return "{{faker.bool}}"
	case "google.protobuf.DoubleValue":
		return "0"
	case "google.protobuf.FloatValue":
		return "0"
	case "google.protobuf.Int64Value", "google.protobuf.UInt64Value", "google.protobuf.Int32Value", "google.protobuf.UInt32Value":
		return "{{faker.int}}"
	default:
		return b.buildMessage(md, depth)
	}
}

func (b *templateBuilder) guessStringToken(fieldName string) string {
	n := strings.ToLower(fieldName)

	if strings.Contains(n, "email") {
		return "{{faker.email}}"
	}
	if strings.Contains(n, "phone") || strings.Contains(n, "mobile") {
		return "{{faker.phone}}"
	}
	if strings.Contains(n, "username") {
		return "{{faker.username}}"
	}

	if n == "name" || strings.Contains(n, "full_name") {
		return "{{faker.name}}"
	}
	if strings.Contains(n, "first_name") || n == "firstname" {
		return "{{faker.firstName}}"
	}
	if strings.Contains(n, "last_name") || n == "lastname" {
		return "{{faker.lastName}}"
	}

	if n == "id" || strings.HasSuffix(n, "_id") || strings.Contains(n, "user_id") {
		return "{{faker.uuid}}"
	}

	if strings.Contains(n, "timestamp") ||
		strings.Contains(n, "created_at") ||
		strings.Contains(n, "updated_at") ||
		strings.Contains(n, "time") {
		return "{{faker.timestamp}}"
	}

	if strings.Contains(n, "message") || strings.Contains(n, "text") || strings.Contains(n, "description") {
		return "{{faker.sentence}}"
	}

	return b.safeToken
}
