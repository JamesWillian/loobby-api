package app.loobby.auth.dto

/**
 * Body do POST /auth/google.
 *
 * - [idToken]: ID token do Google Sign-In (valida identidade Google).
 * - [firebaseIdToken]: opcional para compat; quando presente, o telefone
 *   verificado via Firebase Phone Auth é vinculado/mesclado à conta.
 *   Ver comentário em [app.loobby.auth.dto.RegisterRequest].
 */
data class GoogleAuthRequest(
    val idToken: String,
    val firebaseIdToken: String? = null
)