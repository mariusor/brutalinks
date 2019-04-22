package tests

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"testing"
)

// {"id": "id-rsa",
// "prv": "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCvXPioxuVzfs9IvTudQ/SnJl7PbD7uJnIY9HC15bMvQg/Y9rr2hhztS5rNxjq8nawMtSjadTUZao5UC6E61jO4QYYCxBzrahpdk0B3v0WzX14fpN2h4gvQFcAtEt1xSEJ8PoRoahF93/FH2/wNphQ1FDBrhYmKs5f3k6+T1g8l6OvtFUkoasrVbpRHjAiY+fQCyXdimS96N9R2VF9L9T8iqNjwLXuRpwwFS16KfOgajN//c5k8/jFlkcJnzkgqzHMIh7hZave5VVc/behAvT+SMoHBAtMTSygrdkbhQ+gO95HQKjfOoZiiP+ryLh2bXHXyX4RaKfb31N83Ep86L/9bAgMBAAECggEBAKNqcA5XytqmAWQ3c6ZJ/WMGTrPcm4gyK4E1yRK4yxHu7fWxdujkcXBwVAIOCA5coEf3SerJ7oGQ2rFXZRf/JJM//DH3rztx1L/+yMTOaZWN+ZhjemWw0HFI050tR06Zl9tQJvNmZIZ4edANIAVYDtynw7du6Y1nbuY3qhaKE/OuXs1D5mc2C/YcO1/jqK8IRg6ig3BzYNctvaRujfkwYdpbAU/C/NPE1JhxcczCEV4V6hX8Rh86/a8gAjOJo3Vpao2Yn/Av8S9gwJJSeqCnjFjjltpbhYInwug6qE7w8IyQOce9lmVNAsQhyP2b7+TmrTjJ5GdIMhBHR3MN1VJ79oECgYEAxXtN43Uu8PkRbLMcWamNqlgBXSU43LO11kmBA6syvGvKNh37HjKkFT5883kP37e8JJmu/jGVRSQT0cCQ/0LCpI9N6ZVo4/Qy41xTiO1yCt76nfcz6axmnh/NkQuPw55XzJSiIlNae3Mo+erPas094Tpe7i0grsLS4y4kvQnmIf0CgYEA41PJurI+Hjg2lYVqwFybNtvoJwmSJlT3iyTLGTKj8r2W9UUiSFm0y1Pkky639Lv8W/n2Yv6HLwN2n8EC1sKPaCo0uIdyJIo9jGvXuBxbC+uwrlfqi2G9asquEtGBtYTxTIsLNd0MmLgXWyItNmPJ3VsmW57MaW/9H4mtqfvnGjcCgYADpWUov+8f79lMgnoRhbnh3UIZMCi+mmrPDAhfwvdq1yqimScbxZ+V7NNtw1xxqvjETDoY4114K1RaWQ3USK1DUIoFuAZ5vvZ5kCjSrF+gp8FEzV2eANrcLIYlGWuMFw5T7qKXs6ZGBThKdPVjaWqtD+DU0Ox7jYlLPHzdKKOhGQKBgEo5E0an5+xKAlhzhVVlZzBUQMpuL4/gciq2SrHhfPJGwME4X2csEwHIVHtR57I6pr0JMk9EN7M7+EFU7a5dPCGQlkIhxzzy/gGZPIfaikesHrXt8qCruwLhRlDSov03eOm7BGAD2pcKlGWnFQgLMN/bYPzNLoTGkej4NQGpQ92lAoGBAJTQMYFAEtBbGiU2dZiSpbs1zO7cX/O9MMyJ+n81bMIJ/7N7vPzHbn6ZCCissKSb/iPx+tRQPbLD2+UWNum01Z8hlBzJrinRr+1PayC/8rbSSXujrHPdpfVTXuc/hIZ+Jq2qtIm9oUra2zvpg7t8pVp/7JDLVflkiHqDT3aSBuq3",

var lpubblock, _ = pem.Decode([]byte("-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAr1z4qMblc37PSL07nUP0pyZez2w+7iZyGPRwteWzL0IP2Pa69oYc7UuazcY6vJ2sDLUo2nU1GWqOVAuhOtYzuEGGAsQc62oaXZNAd79Fs19eH6TdoeIL0BXALRLdcUhCfD6EaGoRfd/xR9v8DaYUNRQwa4WJirOX95Ovk9YPJejr7RVJKGrK1W6UR4wImPn0Asl3YpkvejfUdlRfS/U/IqjY8C17kacMBUteinzoGozf/3OZPP4xZZHCZ85IKsxzCIe4WWr3uVVXP23oQL0/kjKBwQLTE0soK3ZG4UPoDveR0Co3zqGYoj/q8i4dm1x18l+EWin299TfNxKfOi//WwIDAQAB\n-----END PUBLIC KEY-----"))
var lpub, _ = x509.ParsePKIXPublicKey(lpubblock.Bytes)
var lprvblock, _ = pem.Decode([]byte("-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCvXPioxuVzfs9IvTudQ/SnJl7PbD7uJnIY9HC15bMvQg/Y9rr2hhztS5rNxjq8nawMtSjadTUZao5UC6E61jO4QYYCxBzrahpdk0B3v0WzX14fpN2h4gvQFcAtEt1xSEJ8PoRoahF93/FH2/wNphQ1FDBrhYmKs5f3k6+T1g8l6OvtFUkoasrVbpRHjAiY+fQCyXdimS96N9R2VF9L9T8iqNjwLXuRpwwFS16KfOgajN//c5k8/jFlkcJnzkgqzHMIh7hZave5VVc/behAvT+SMoHBAtMTSygrdkbhQ+gO95HQKjfOoZiiP+ryLh2bXHXyX4RaKfb31N83Ep86L/9bAgMBAAECggEBAKNqcA5XytqmAWQ3c6ZJ/WMGTrPcm4gyK4E1yRK4yxHu7fWxdujkcXBwVAIOCA5coEf3SerJ7oGQ2rFXZRf/JJM//DH3rztx1L/+yMTOaZWN+ZhjemWw0HFI050tR06Zl9tQJvNmZIZ4edANIAVYDtynw7du6Y1nbuY3qhaKE/OuXs1D5mc2C/YcO1/jqK8IRg6ig3BzYNctvaRujfkwYdpbAU/C/NPE1JhxcczCEV4V6hX8Rh86/a8gAjOJo3Vpao2Yn/Av8S9gwJJSeqCnjFjjltpbhYInwug6qE7w8IyQOce9lmVNAsQhyP2b7+TmrTjJ5GdIMhBHR3MN1VJ79oECgYEAxXtN43Uu8PkRbLMcWamNqlgBXSU43LO11kmBA6syvGvKNh37HjKkFT5883kP37e8JJmu/jGVRSQT0cCQ/0LCpI9N6ZVo4/Qy41xTiO1yCt76nfcz6axmnh/NkQuPw55XzJSiIlNae3Mo+erPas094Tpe7i0grsLS4y4kvQnmIf0CgYEA41PJurI+Hjg2lYVqwFybNtvoJwmSJlT3iyTLGTKj8r2W9UUiSFm0y1Pkky639Lv8W/n2Yv6HLwN2n8EC1sKPaCo0uIdyJIo9jGvXuBxbC+uwrlfqi2G9asquEtGBtYTxTIsLNd0MmLgXWyItNmPJ3VsmW57MaW/9H4mtqfvnGjcCgYADpWUov+8f79lMgnoRhbnh3UIZMCi+mmrPDAhfwvdq1yqimScbxZ+V7NNtw1xxqvjETDoY4114K1RaWQ3USK1DUIoFuAZ5vvZ5kCjSrF+gp8FEzV2eANrcLIYlGWuMFw5T7qKXs6ZGBThKdPVjaWqtD+DU0Ox7jYlLPHzdKKOhGQKBgEo5E0an5+xKAlhzhVVlZzBUQMpuL4/gciq2SrHhfPJGwME4X2csEwHIVHtR57I6pr0JMk9EN7M7+EFU7a5dPCGQlkIhxzzy/gGZPIfaikesHrXt8qCruwLhRlDSov03eOm7BGAD2pcKlGWnFQgLMN/bYPzNLoTGkej4NQGpQ92lAoGBAJTQMYFAEtBbGiU2dZiSpbs1zO7cX/O9MMyJ+n81bMIJ/7N7vPzHbn6ZCCissKSb/iPx+tRQPbLD2+UWNum01Z8hlBzJrinRr+1PayC/8rbSSXujrHPdpfVTXuc/hIZ+Jq2qtIm9oUra2zvpg7t8pVp/7JDLVflkiHqDT3aSBuq3\n-----END PRIVATE KEY-----"))
var lprv, _ = x509.ParsePKCS8PrivateKey(lprvblock.Bytes)

var littrAcct = testAccount{
	id:         "https://littr.me/api/self/following/64e55785250f59637e7f578167e2c112",
	Hash:       "64e55785250f59637e7f578167e2c112",
	Handle:     "0lpbm",
	publicKey:  lpub.(*rsa.PublicKey),
	privateKey: lprv.(*rsa.PrivateKey),
}

var S2STests = testPairs{
	"Follow_SelfService": {{
		req: testReq{
			met:     http.MethodPost,
			url:     inboxURL,
			account: &littrAcct,
			body: fmt.Sprintf(`{
 "type": "Follow",
 "actor": "%s",
 "to": ["%s/self"]
}`, littrAcct.id, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
}

func Test_S2SRequests(t *testing.T) {
	testSuite(t, S2STests)
}
