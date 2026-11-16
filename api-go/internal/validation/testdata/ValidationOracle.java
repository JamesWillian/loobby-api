// Gera o gabarito do pacote validation a partir do Hibernate Validator REAL
// (8.0.3, o do app): os padroes do validador de email, as mensagens de cada
// idioma ja resolvidas e o veredito de email para uma lista de amostras.
// Nao roda no `go test`: o resultado fica versionado em oracle.json.
//
//   java -cp "<classpath do app>" ValidationOracle.java > oracle.json
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.lang.reflect.Field;
import java.util.List;
import java.util.Locale;
import java.util.ResourceBundle;
import java.util.regex.Pattern;
import org.hibernate.validator.internal.constraintvalidators.AbstractEmailValidator;
import org.hibernate.validator.internal.constraintvalidators.bv.EmailValidator;
import org.hibernate.validator.internal.util.DomainNameUtil;

public class ValidationOracle {
    static String pattern(Class<?> c, String field) throws Exception {
        Field f = c.getDeclaredField(field); f.setAccessible(true);
        Pattern p = (Pattern) f.get(null);
        return p.flags() + "|" + p.pattern();
    }
    public static void main(String[] args) throws Exception {
        ObjectMapper m = new ObjectMapper();
        ObjectNode out = m.createObjectNode();
        out.put("localPart", pattern(AbstractEmailValidator.class, "LOCAL_PART_PATTERN"));
        out.put("domain", pattern(DomainNameUtil.class, "EMAIL_DOMAIN_PATTERN"));

        ObjectNode msgs = out.putObject("messages");
        String[] keys = {"NotBlank", "Email", "Size", "FutureOrPresent", "NotNull", "Positive"};
        for (String tag : List.of("", "ar", "cs", "da", "de", "en", "es", "fa", "fr", "hu", "it", "ja", "ko", "mn-MN",
                "nl", "pl", "pt", "pt-BR", "pt-PT", "ro", "ru", "sk", "tr", "uk", "zh", "zh-CN", "zh-TW")) {
            Locale loc = tag.isEmpty() ? Locale.ROOT : Locale.forLanguageTag(tag);
            ResourceBundle b = ResourceBundle.getBundle("org.hibernate.validator.ValidationMessages", loc,
                    ResourceBundle.Control.getNoFallbackControl(ResourceBundle.Control.FORMAT_PROPERTIES));
            ObjectNode n = msgs.putObject(tag.replace('-', '_'));
            for (String k : keys) n.put(k, b.getString("jakarta.validation.constraints." + k + ".message"));
        }

        ObjectNode emails = out.putObject("emails");
        EmailValidator v = new EmailValidator();
        StringBuilder l64 = new StringBuilder(), l65 = new StringBuilder(), lab63 = new StringBuilder(), lab64 = new StringBuilder();
        for (int i = 0; i < 64; i++) l64.append('a');
        for (int i = 0; i < 65; i++) l65.append('a');
        for (int i = 0; i < 63; i++) lab63.append('b');
        for (int i = 0; i < 64; i++) lab64.append('b');
        for (String e : List.of("", " ", "user@loobby.app", "USER@LOOBBY.APP", "a@b", "a@b.c", "nao-e-email", "@b.com", "a@",
                "a b@c.com", "a@b c.com", "a..b@c.com", ".a@c.com", "a.@c.com", "a@c..com", "a@.c.com", "a@c.com.",
                "a@-c.com", "a@c-.com", "a@c-d.com", "a@c--d.com", "a+tag@c.com", "o'brien@c.com", "a@b@c.com",
                "\"a b\"@c.com", "\"a@b\"@c.com", "a@[127.0.0.1]", "a@[IPv6:::1]", "a@127.0.0.1", "a@localhost",
                "joão@exemplo.com", "a@exémplo.com", "a@münchen.de", "😀@c.com", "a@😀.com", "a@c.com\n", " a@c.com",
                "a@c.com ", "a@c_d.com", "a_b@c.com", "a@c.d-", "a!#$%&*+/=?^_`{|}~@c.com", "a(b)@c.com", "a,b@c.com",
                "a;b@c.com", "a<b>@c.com", "a[b]@c.com", "a\\b@c.com", "a\"b@c.com", "a@c.com,", "a@1.2", "A@B.C",
                l64 + "@c.com", l65 + "@c.com", "a@" + lab63 + ".com", "a@" + lab64 + ".com", "a@b.c.d.e.f", "contract-suite@loobby.app",
                "a@xn--mnchen-3ya.de", "a@c.123", "a@123.com", "a@c.com.br", "a\tb@c.com", "a@c\t.com")) {
            emails.put(e, v.isValid(e, null));
        }
        System.out.println(m.writerWithDefaultPrettyPrinter().writeValueAsString(out));
    }
}
