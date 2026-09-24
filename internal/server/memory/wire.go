package memory

// These aliases expose the exact types serialized by the live tools for protocol
// schema generation. They do not define a second response model.
type (
	RecallRequest           = recallRequest
	RecallResponse          = recallResponse
	RecallFact              = recallFact
	RecallResult            = recallResult
	RememberRequest         = rememberRequest
	RememberResponse        = rememberResponse
	EntityRequest           = entityRequest
	EntityResponse          = entityResponse
	SynthesizeRequest       = synthesizeRequest
	SynthesizeResponse      = synthesizeResponse
	SynthesizeCost          = synthesizeCost
	CancelOperationRequest  = cancelOperationRequest
	CancelOperationResponse = cancelOperationResponse
	ForgetRequest           = forgetRequest
	ForgetResponse          = forgetResponse
	ReadMemoryFactRequest   = readMemoryFactRequest
	ReadMemoryFactResponse  = readMemoryFactResponse
)
