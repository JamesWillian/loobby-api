package httpx

import "time"

// Instant reproduz o formato que o Jackson usa para java.time.Instant.
//
// O DateTimeFormatter.ISO_INSTANT emite a fracao de segundo em grupos de 0, 3, 6
// ou 9 digitos -- o menor grupo que representa o valor sem perda. Isso foi medido
// na baseline de contrato, onde tres campos saem com precisoes diferentes:
//
//	scheduledDatetime  0 digitos   2026-09-15T16:14:35Z
//	createdAt          6 digitos   2026-09-08T16:14:36.211055Z
//	timestamp (erro)   9 digitos   2026-09-08T16:14:32.551538700Z
//
// Nenhum layout pronto do Go reproduz isso. O RFC3339Nano (padrao do
// time.Time.MarshalJSON) remove TODOS os zeros a direita, entao .211050 viraria
// .21105 -- 5 digitos, um grupo que o Jackson nunca emite. E ".000000000" fixa
// em 9, quebrando os campos de 0 e 6.
type Instant time.Time

func (i Instant) Format() string {
	t := time.Time(i).UTC()
	switch ns := t.Nanosecond(); {
	case ns == 0:
		return t.Format("2006-01-02T15:04:05Z")
	case ns%1_000_000 == 0:
		return t.Format("2006-01-02T15:04:05.000Z")
	case ns%1_000 == 0:
		return t.Format("2006-01-02T15:04:05.000000Z")
	default:
		return t.Format("2006-01-02T15:04:05.000000000Z")
	}
}

func (i Instant) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 34)
	b = append(b, '"')
	b = append(b, i.Format()...)
	b = append(b, '"')
	return b, nil
}

func (i *Instant) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, string(data[1:len(data)-1]))
	if err != nil {
		return err
	}
	*i = Instant(t)
	return nil
}
