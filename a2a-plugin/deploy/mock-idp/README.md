# IdP simulado

Solo para el entorno de demostración y las pruebas. Emite JWT RS256 con el flujo
`client_credentials` y publica su JWKS.

Los clientes y sus secretos los genera `deploy/demo-secrets` en
`deploy/.secrets/idp/clients.json`, que está excluido de git. La clave de firma se
genera en memoria al arrancar. En producción, sustitúyelo por el IdP de la
organización y configura `auth/validator` con su `jwk_url`.
