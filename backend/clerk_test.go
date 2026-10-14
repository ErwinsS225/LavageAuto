package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Ces tests couvrent la vérification des jetons Clerk sans réseau : la clé
// publique statique (JWKS_PUBLIC_KEY) doit permettre de valider une session,
// et un jeton signé par une autre clé doit être refusé.

// cleTestPEM fabrique une paire RSA jetable et renvoie la clé privée et la
// clé publique au format PEM, comme celle fournie par Clerk.
func cleTestPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	prive, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("génération de la clé de test : %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&prive.PublicKey)
	if err != nil {
		t.Fatalf("encodage de la clé publique : %v", err)
	}
	pemPublique := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	return prive, pemPublique
}

func jetonTest(t *testing.T, prive *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	jeton := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signe, err := jeton.SignedString(prive)
	if err != nil {
		t.Fatalf("signature du jeton de test : %v", err)
	}
	return signe
}

func TestClePubliqueStatiqueVerifieLeJeton(t *testing.T) {
	prive, pemPublique := cleTestPEM(t)

	cle, err := parsePublicKeyPEM(pemPublique)
	if err != nil {
		t.Fatalf("lecture de la clé publique PEM : %v", err)
	}

	// État équivalent à celui posé par initClerk quand aucune adresse JWKS
	// n'est configurée : seule la clé statique est disponible.
	sauvegardeKeyfunc, sauvegardeStatique := clerkKeyfunc, clerkStaticKey
	clerkKeyfunc, clerkStaticKey = nil, cle
	t.Cleanup(func() { clerkKeyfunc, clerkStaticKey = sauvegardeKeyfunc, sauvegardeStatique })

	jeton := jetonTest(t, prive, jwt.MapClaims{
		"sub": "user_2abc",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	sub, _, err := verifyClerkSessionToken(jeton)
	if err != nil {
		t.Fatalf("jeton valide refusé : %v", err)
	}
	if sub != "user_2abc" {
		t.Fatalf("subject attendu user_2abc, obtenu %q", sub)
	}
}

func TestJetonSigneParUneAutreCleEstRefuse(t *testing.T) {
	_, pemPublique := cleTestPEM(t)
	cle, err := parsePublicKeyPEM(pemPublique)
	if err != nil {
		t.Fatalf("lecture de la clé publique PEM : %v", err)
	}

	sauvegardeKeyfunc, sauvegardeStatique := clerkKeyfunc, clerkStaticKey
	clerkKeyfunc, clerkStaticKey = nil, cle
	t.Cleanup(func() { clerkKeyfunc, clerkStaticKey = sauvegardeKeyfunc, sauvegardeStatique })

	// Clé différente : le jeton ne doit pas passer.
	autre, _ := cleTestPEM(t)
	jeton := jetonTest(t, autre, jwt.MapClaims{
		"sub": "usurpateur",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, _, err := verifyClerkSessionToken(jeton); err == nil {
		t.Fatal("un jeton signé par une autre clé a été accepté")
	}
}

func TestJetonExpireEstRefuse(t *testing.T) {
	prive, pemPublique := cleTestPEM(t)
	cle, err := parsePublicKeyPEM(pemPublique)
	if err != nil {
		t.Fatalf("lecture de la clé publique PEM : %v", err)
	}

	sauvegardeKeyfunc, sauvegardeStatique := clerkKeyfunc, clerkStaticKey
	clerkKeyfunc, clerkStaticKey = nil, cle
	t.Cleanup(func() { clerkKeyfunc, clerkStaticKey = sauvegardeKeyfunc, sauvegardeStatique })

	jeton := jetonTest(t, prive, jwt.MapClaims{
		"sub": "user_perime",
		"exp": time.Now().Add(-time.Minute).Unix(),
	})

	if _, _, err := verifyClerkSessionToken(jeton); err == nil {
		t.Fatal("un jeton expiré a été accepté")
	}
}

func TestRepliSurLaCleStatiqueQuandLaJWKSSeTait(t *testing.T) {
	prive, pemPublique := cleTestPEM(t)
	cle, err := parsePublicKeyPEM(pemPublique)
	if err != nil {
		t.Fatalf("lecture de la clé publique PEM : %v", err)
	}

	// JWKS « présente mais muette » : c'est le cas d'une station dont le
	// réseau vacille — l'appel échoue, le jeton doit tout de même être validé
	// par la clé publique fournie en configuration.
	sauvegardeKeyfunc, sauvegardeStatique := clerkKeyfunc, clerkStaticKey
	clerkKeyfunc = func(*jwt.Token) (any, error) {
		return nil, fmt.Errorf("JWKS injoignable")
	}
	clerkStaticKey = cle
	t.Cleanup(func() { clerkKeyfunc, clerkStaticKey = sauvegardeKeyfunc, sauvegardeStatique })

	jeton := jetonTest(t, prive, jwt.MapClaims{
		"sub": "user_reseau_instable",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	sub, _, err := verifyClerkSessionToken(jeton)
	if err != nil {
		t.Fatalf("repli sur la clé statique attendu, erreur : %v", err)
	}
	if sub != "user_reseau_instable" {
		t.Fatalf("subject attendu user_reseau_instable, obtenu %q", sub)
	}
}

func TestClePubliqueIllisible(t *testing.T) {
	for _, contenu := range []string{"", "pas une clé", "-----BEGIN PUBLIC KEY-----\nnawak\n-----END PUBLIC KEY-----"} {
		if _, err := parsePublicKeyPEM(contenu); err == nil {
			t.Fatalf("clé publique invalide acceptée : %q", contenu)
		}
	}
}
