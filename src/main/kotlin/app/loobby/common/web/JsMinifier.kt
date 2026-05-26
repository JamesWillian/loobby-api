package app.loobby.common.web

/**
 * Abstração de minificador de JavaScript. Existe como interface (DIP +
 * Open/Closed) pra deixar fácil trocar de provedor — hoje é Closure Compiler,
 * amanhã pode ser Terser, esbuild via Node, ou nenhum (no-op) em dev.
 */
interface JsMinifier {
    /**
     * Minifica o código JS recebido. Em caso de erro de parsing, a
     * implementação DEVE devolver o código original em vez de propagar a
     * exceção — minificação é uma otimização, não pode quebrar o app.
     */
    fun minify(source: String): String
}
