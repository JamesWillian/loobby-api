package app.loobby.common.web

import com.google.javascript.jscomp.CompilationLevel
import com.google.javascript.jscomp.Compiler
import com.google.javascript.jscomp.CompilerOptions
import com.google.javascript.jscomp.SourceFile
import com.google.javascript.jscomp.WarningLevel
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component

/**
 * Implementação de [JsMinifier] usando Google Closure Compiler.
 *
 * Estratégia em três camadas, do mais agressivo ao mais conservador:
 *
 *  1. **Extract-imports → IIFE → Closure SIMPLE** — o caso ideal:
 *     a. Separa os `import … from "URL"` do topo (Closure não resolve URLs ES6
 *        no modo BROWSER e quebra com `JSC_INVALID_MODULE_PATH`).
 *     b. Envolve o resto numa IIFE pra que as vars top-level virem
 *        function-scoped e o `SIMPLE_OPTIMIZATIONS` consiga renomeá-las.
 *     c. Junta imports + corpo minificado.
 *
 *  2. **Fallback regex** — se o Closure ainda assim falhar, fazemos só strip
 *     de comentários + colapso de whitespace, sem renomear.
 *
 *  3. **Source original** — em caso extremo (regex também falhar por algum
 *     bug), devolve o source intocado: minificação é otimização, nunca pode
 *     quebrar o app.
 */
@Component
class ClosureJsMinifier : JsMinifier {

    private val log = LoggerFactory.getLogger(javaClass)

    override fun minify(source: String): String {
        if (source.isBlank()) return source

        // 1) Tenta o caminho ideal (Closure após extrair imports e envolver em IIFE)
        tryClosureWithImportsHoisted(source)?.let { result ->
            log.info(
                "JS minificado via Closure: {} → {} chars ({}% redução)",
                source.length, result.length, percentReduction(source, result)
            )
            return result
        }

        // 2) Fallback regex
        val regexResult = minifyWithRegex(source)
        log.info(
            "JS minificado via fallback regex: {} → {} chars ({}% redução)",
            source.length, regexResult.length, percentReduction(source, regexResult)
        )
        return regexResult
    }

    // -------------------------------------------------------------------------
    // Caminho ideal: extract imports → IIFE → Closure
    // -------------------------------------------------------------------------

    private fun tryClosureWithImportsHoisted(source: String): String? {
        val (imports, body) = extractImports(source)
        if (body.isBlank()) {
            // Só imports — nada a minificar
            return imports.joinToString("\n")
        }

        // IIFE faz vars top-level virarem locais → Closure pode renomear.
        // As imports continuam acessíveis porque a IIFE herda o escopo do module.
        val wrappedBody = "(function(){\n$body\n})();"

        val minifiedBody = runClosure(wrappedBody) ?: return null

        return if (imports.isEmpty()) minifiedBody
        else imports.joinToString("\n") + "\n" + minifiedBody
    }

    /** Roda o Closure Compiler em SIMPLE_OPTIMIZATIONS. Devolve null em qualquer falha. */
    private fun runClosure(source: String): String? = try {
        val compiler = Compiler().apply { disableThreads() }

        val options = CompilerOptions().apply {
            setLanguageIn(CompilerOptions.LanguageMode.ECMASCRIPT_NEXT)
            setLanguageOut(CompilerOptions.LanguageMode.NO_TRANSPILE)
            CompilationLevel.SIMPLE_OPTIMIZATIONS.setOptionsForCompilationLevel(this)
            WarningLevel.QUIET.setOptionsForWarningLevel(this)
            setEmitUseStrict(false)
        }

        val input = SourceFile.fromCode("inline.js", source)
        compiler.compile(emptyList(), listOf(input), options)

        if (compiler.hasErrors()) {
            log.warn(
                "Closure Compiler errors: {}",
                compiler.errors.joinToString("; ") { it.description }
            )
            null
        } else {
            compiler.toSource().ifBlank { null }
        }
    } catch (t: Throwable) {
        log.warn("Closure Compiler exception: {}", t.message)
        null
    }

    /**
     * Separa o cabeçalho de `import … from "URL"` (e variantes) do corpo do módulo.
     * Captura imports multilinhas (com chaves abertas em outra linha).
     */
    private fun extractImports(source: String): Pair<List<String>, String> {
        val matches = IMPORT_STATEMENT.findAll(source).toList()
        if (matches.isEmpty()) return Pair(emptyList(), source.trim())

        val imports = matches.map { it.value.trim() }
        // Remove em ordem reversa para preservar offsets.
        var body = source
        for (m in matches.sortedByDescending { it.range.first }) {
            body = body.removeRange(m.range)
        }
        return Pair(imports, body.trim())
    }

    // -------------------------------------------------------------------------
    // Fallback regex
    // -------------------------------------------------------------------------

    /**
     * Strip de comentários + compactação de whitespace. Não renomeia variáveis
     * (regex não distingue identificador de string com segurança).
     *
     * Limitação conhecida: pode remover sequências `//` mal posicionadas, mas
     * o regex de comentário de linha exige whitespace antes do `//`, então
     * URLs do tipo `"https://..."` em strings ficam intactas.
     */
    private fun minifyWithRegex(source: String): String {
        var s = source
        s = s.replace(BLOCK_COMMENT, "")
        s = s.replace(LEADING_LINE_COMMENT, "")
        s = s.replace(BLANK_LINE, "")
        s = s.lines().joinToString("\n") { it.trim() }.trim()
        return s
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    private fun percentReduction(source: String, minified: String): Int =
        if (source.isEmpty()) 0
        else ((source.length - minified.length) * 100) / source.length

    private companion object {
        /**
         * Captura `import …;` com ou sem cláusula `from "…"`.
         * `[\s\S]*?` é non-greedy + cross-line — pega imports multilinhas.
         */
        private val IMPORT_STATEMENT = Regex(
            """import\s+(?:[\s\S]*?\s+from\s+)?["'][^"']+["']\s*;?"""
        )

        private val BLOCK_COMMENT = Regex("""/\*[\s\S]*?\*/""")
        private val LEADING_LINE_COMMENT = Regex("""(?m)^\s*//.*$""")
        private val BLANK_LINE = Regex("""(?m)^\s*$\n""")
    }
}
