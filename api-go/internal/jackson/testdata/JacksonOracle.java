// Gera o gabarito do pacote jackson a partir do Jackson REAL, na versao do app.
// Nao roda no `go test`: o resultado fica versionado em oracle.jsonl.
//
//   G=~/.gradle/caches/modules-2/files-2.1/com.fasterxml.jackson.core
//   CP=$(ls $G/jackson-{core,databind,annotations}/2.19.2/*/*-2.19.2.jar | paste -sd: -)
//   java -cp "$CP" internal/jackson/testdata/JacksonOracle.java > internal/jackson/testdata/oracle.jsonl
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.DoubleNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.ArrayList;
import java.util.List;
import java.util.Random;

public class JacksonOracle {
    static final ObjectMapper M = new ObjectMapper();

    static void emit(ObjectNode n) throws Exception { System.out.println(M.writeValueAsString(n)); }

    public static void main(String[] args) throws Exception {
        // 1) Double.toString e a escrita de um DoubleNode (genres/platforms)
        List<Double> ds = new ArrayList<>(List.of(0.0, -0.0, 4.38, 4.5, 4.0, 0.001, 0.000999,
            1e7, 9999999.0, 1.234e-5, 0.1 + 0.2, 1.0 / 3, 2e23, 5e-324, Double.MAX_VALUE,
            100.0, 123456.789, 3.31, 0.05, 1e21, 1e-7, 123e10, -4.38, 4.99, 3.0, 1e-3));
        Random r = new Random(42);
        for (int i = 0; i < 1500; i++) {
            double d = Double.longBitsToDouble(r.nextLong());
            if (!Double.isNaN(d) && !Double.isInfinite(d)) ds.add(d);
        }
        for (int i = 0; i < 1500; i++) ds.add(Math.round(r.nextDouble() * 500) / 100.0); // notas 0-5
        for (double d : ds) {
            ObjectNode n = M.createObjectNode();
            n.put("kind", "double");
            n.put("bits", Long.toHexString(Double.doubleToRawLongBits(d)));
            n.put("java", Double.toString(d));
            n.put("node", M.writeValueAsString(DoubleNode.valueOf(d)));
            emit(n);
        }

        // 2) literal do RAWG -> isNumber/isInt/decimalValue(), como o GameService le o rating
        for (String lit : List.of("0", "0.0", "0.00", "4", "4.0", "4.50", "4.38", "-0", "3.31",
                "1e2", "1E7", "1.234e-5", "12345678901234567890", "2147483647", "2147483648",
                "-2147483648", "-2147483649", "5.0", "100.0", "0.001", "0.0001", "1e-7")) {
            JsonNode node = M.readTree(lit);
            ObjectNode n = M.createObjectNode();
            n.put("kind", "decimal");
            n.put("lit", lit);
            n.put("out", M.writeValueAsString(node.decimalValue()));
            n.put("isInt", node.isInt());
            emit(n);
        }

        // 3) readTree + escrita: o que genres/platforms viram no caminho do Kotlin
        for (String json : List.of(
                "{\"slug\":\"a\",\"id\":1,\"name\":\"x\"}",
                "{ \"a\" : 1 , \"b\" : [ 1 , 2.50 , -0 , 1e2 , true , null ] }",
                "{\"k\":1,\"k\":2,\"z\":3}",
                "[]", "{}", "[{}]", "\"so uma string\"", "42", "4.50", "null",
                "[1.0, 2.25, 12345678901234567890, 1E-10, 0.000999]",
                "{\"t\":\"aspas \\\" barra \\\\ nova\\nlinha tab\\t cr\\r bs\\b ff\\f\"}",
                "{\"u\":\"\\u0001\\u001f\\u007f \\u2028 \\u2029 \\u00e9 \\ud83d\\ude00 / \\/\"}",
                "{\"platforms\":[{\"platform\":{\"id\":4,\"name\":\"PC\",\"slug\":\"pc\",\"image\":null,\"year_end\":null,\"year_start\":null,\"games_count\":556081,\"image_background\":\"https://media.rawg.io/x.jpg\"},\"released_at\":\"2017-03-22\",\"requirements\":{\"minimum\":\"Minimum:\\nOS: Windows 7\"}}]}")) {
            ObjectNode n = M.createObjectNode();
            n.put("kind", "normalize");
            n.put("in", json);
            n.put("out", M.writeValueAsString(M.readTree(json)));
            emit(n);
        }

        // 4) escape de string: todo o ASCII de controle, mais os casos que divergem do Go
        StringBuilder all = new StringBuilder();
        for (char c = 0; c < 0x80; c++) all.append(c);
        for (String s : List.of(all.toString(), "\u2028\u2029", "ação ñ 日本 😀", "<&>", "a/b")) {
            ObjectNode n = M.createObjectNode();
            n.put("kind", "string");
            n.put("in", s);
            n.put("out", M.writeValueAsString(s));
            emit(n);
        }
    }
}
