package agents

const ExtractionSchema = `{"type":"object","additionalProperties":false,"required":["facts"],"properties":{"facts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["statement","quote"],"properties":{"statement":{"type":"string"},"quote":{"type":"string"}}}}}}`

const ReviewSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["level", "verdict", "rationale", "evidence", "validation_minimum"],
  "properties": {
    "level": {"type": "string", "enum": ["D1", "D2", "D3"]},
    "verdict": {"type": "string", "enum": ["approve", "revise", "needs_owner"]},
    "rationale": {"type": "string"},
    "evidence": {"type": "string"},
    "validation_minimum": {"type": "string", "enum": ["", "V1", "V2", "V3", "V4"]}
  }
}`
