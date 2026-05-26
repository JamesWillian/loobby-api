package app.loobby.auth.dto

import jakarta.validation.constraints.Email
import jakarta.validation.constraints.NotBlank
import jakarta.validation.constraints.Size

/**
 * Body do POST /auth/register.
 *
 * O campo [firebaseIdToken] é opcional para garantir compatibilidade com
 * versões mais antigas do app Android, mas torna-se obrigatório no fluxo
 * de UI assim que o app passa a exigir telefone no registro. Quando presente:
 *  - o token é validado via Firebase Auth (Admin SDK)
 *  - o telefone é extraído e vinculado ao usuário
 *  - se outro usuário "lite" (criado por confirmação via link público) já
 *    possuir esse telefone, a conta lite é mesclada ao usuário corrente
 *  - se o telefone já pertencer a outra conta full, o registro é rejeitado
 */
data class RegisterRequest(
    @field:Email
    @field:NotBlank
    val email: String,

    @field:NotBlank
    @field:Size(min = 6, max = 100)
    val password: String,

    val firebaseIdToken: String? = null
)
