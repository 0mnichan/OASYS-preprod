package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/playwright-community/playwright-go"
)

var srmPage playwright.Page
var captchaPath = "./captcha.jpg"

// Install Playwright and required browsers
func installPlaywright() {
	log.Println("Installing Playwright and browsers...")
	cmd := exec.Command("sh", "-c", "npm install -g playwright && npx playwright install")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to install Playwright: %v\nOutput: %s", err, string(output))
	}
	log.Println("Playwright installed successfully.")
}

func main() {
	installPlaywright()

	// Initialize Playwright
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Could not start Playwright: %v", err)
	}
	defer pw.Stop()

	// Launch a headless browser
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		log.Fatalf("Could not launch browser: %v", err)
	}
	defer browser.Close()

	// Create a browser context
	context, err := browser.NewContext()
	if err != nil {
		log.Fatalf("Could not create browser context: %v", err)
	}

	// Open a new page for the SRM login
	srmPage, err = context.NewPage()
	if err != nil {
		log.Fatalf("Could not create SRM page: %v", err)
	}

	// Load the initial SRM login page
	_, err = srmPage.Goto("https://sp.srmist.edu.in/srmiststudentportal/students/loginManager/youLogin.jsp", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		log.Fatalf("Could not navigate to SRM login page: %v", err)
	}

	// Capture the initial CAPTCHA
	captureCaptcha()

	// Define HTTP handlers
	http.HandleFunc("/", serveLoginPage)
	http.HandleFunc("/captcha.jpg", serveCaptchaImage)
	http.HandleFunc("/reload_captcha", reloadCaptcha)
	http.HandleFunc("/submit_login", submitLogin)

	// Start the HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}
	log.Printf("Server running at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Capture the CAPTCHA and save it locally
func captureCaptcha() {
	captchaElement, err := srmPage.QuerySelector("img[src*='captchas']")
	if err != nil || captchaElement == nil {
		log.Fatalf("Failed to find CAPTCHA image: %v", err)
	}

	_, err = captchaElement.Screenshot(playwright.ElementHandleScreenshotOptions{
		Path: playwright.String(captchaPath),
	})
	if err != nil {
		log.Fatalf("Could not take CAPTCHA screenshot: %v", err)
	}

	log.Println("CAPTCHA screenshot saved as captcha.jpg")
}

// Serve the login page
func serveLoginPage(w http.ResponseWriter, r *http.Request) {
	timestamp := time.Now().Unix()
	tmpl, err := template.New("login").Parse(`<!DOCTYPE html>
	<html lang="en">
	<head>
	    <meta charset="UTF-8">
	    <title>OASYS Login</title>
	    <script>
	        function reloadCaptcha() {
	            fetch('/reload_captcha', { method: 'POST' })
	                .then(response => {
	                    if (response.ok) {
	                        const captchaImage = document.querySelector("img[src='/captcha.jpg']");
	                        captchaImage.src = "/captcha.jpg?ts=" + new Date().getTime(); // Prevent caching
	                    } else {
	                        alert("Failed to reload CAPTCHA");
	                    }
	                })
	                .catch(err => alert("Error: " + err));
	        }
	    </script>
	</head>
	<body>
	    <h2>OASYS Login</h2>
	    <form action="/submit_login" method="POST">
	        <label for="netid">NetID:</label>
	        <input type="text" id="netid" name="netid" required>
	        <label for="password">Password:</label>
	        <input type="password" id="password" name="password" required>
	        <label for="captcha">CAPTCHA:</label>
	        <input type="text" id="captcha" name="captcha" required>
	        <br>
	        <img src="/captcha.jpg?ts={{.}}" alt="CAPTCHA Image" style="border: 1px solid #000;">
	        <button type="button" onclick="reloadCaptcha()">Reload CAPTCHA</button>
	        <button type="submit">Login</button>
	    </form>
	</body>
	</html>`)
	if err != nil {
		http.Error(w, "Error generating login page", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, timestamp)
}

// Serve the CAPTCHA image
func serveCaptchaImage(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(captchaPath)
	if err != nil {
		log.Printf("Could not open CAPTCHA image: %v", err)
		http.Error(w, "CAPTCHA image not found", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	http.ServeFile(w, r, captchaPath)
}

// Reload the CAPTCHA
func reloadCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, err := srmPage.Goto("https://sp.srmist.edu.in/srmiststudentportal/students/loginManager/youLogin.jsp", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		http.Error(w, "Failed to reload SRM page", http.StatusInternalServerError)
		return
	}

	captureCaptcha()
	w.WriteHeader(http.StatusOK)
}

// Handle login form submission
func submitLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	netid := r.FormValue("netid")
	password := r.FormValue("password")
	captcha := r.FormValue("captcha")

	if err := srmPage.Fill("#login", netid); err != nil {
		http.Error(w, "Could not fill NetID", http.StatusInternalServerError)
		return
	}
	if err := srmPage.Fill("#passwd", password); err != nil {
		http.Error(w, "Could not fill password", http.StatusInternalServerError)
		return
	}
	if err := srmPage.Fill("#ccode", captcha); err != nil {
		http.Error(w, "Could not fill CAPTCHA", http.StatusInternalServerError)
		return
	}

	if err := srmPage.Click("button.btn-custom.btn-user.btn-block.lift"); err != nil {
		http.Error(w, "Could not click login button", http.StatusInternalServerError)
		return
	}

	time.Sleep(3 * time.Second)
	w.Write([]byte("Login submitted successfully."))
}

// Utility for margin calculations
type Margin struct {
	Hours   int
	Message string
}

func calculateMargin(att int, tot int) Margin {
	initAtp := (float32(att) / float32(tot)) * 100
	if initAtp >= 76 {
		n := 0
		for {
			atp := (float32(att) / float32(tot+n)) * 100
			if atp <= 76 {
				return Margin{Hours: n - 1, Message: fmt.Sprintf("You can miss %d hours to stay above 76%%.", n-1)}
			}
			n++
		}
	} else {
		n := 0
		for {
			atp := (float32(att+n) / float32(tot+n)) * 100
			if atp >= 75 {
				return Margin{Hours: n, Message: fmt.Sprintf("You need to attend %d hours to reach 75%%.", n)}
			}
			n++
		}
	}
}
