package httpx

// Number e o texto exato de um numero JSON. Serve para os BigDecimal do Kotlin
// (pricePerPlayer), cujo texto ja vem pronto -- do Postgres, com a escala da
// coluna ("25.50", "0.00") -- e nao pode passar por float64, que o reescreveria
// como 25.5 e 0.
type Number string

func (n Number) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte("null"), nil
	}
	return []byte(n), nil
}
