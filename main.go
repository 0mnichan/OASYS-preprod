package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/playwright-community/playwright-go"
)

var srmPage playwright.Page
var captchaPath = "./captcha.jpg"

func main() {
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
	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Capture CAPTCHA and save it to captchaPath
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
	    <title>OASYS</title>
	    <script>
	        function reloadCaptcha() {
	            fetch('/reload_captcha', { method: 'POST' })
	                .then(response => {
	                    if (response.ok) {
	                        const captchaImage = document.querySelector("img[src='/captcha.jpg']");
	                        captchaImage.src = "/captcha.jpg?ts=" + new Date().getTime(); // Add timestamp to prevent caching
	                    } else {
	                        alert("Failed to reload CAPTCHA");
	                    }
	                })
	                .catch(err => alert("Please reload the page"));
	        }
	    </script>
	</head>
	<body>
	    <h2>OASYS login</h2>
	    <form action="/submit_login" method="POST">
	        <label for="netid">NetID (without '@srmist.edu.in')</label>
	        <input type="text" id="netid" name="netid" required>
	        <label for="password">Password</label>
	        <input type="password" id="password" name="password" required>
	        <label for="captcha">Enter CAPTCHA</label>
	        <input type="text" id="captcha" name="captcha" required>
	        <br>
	        <img src="/captcha.jpg?ts={{.}}" alt="CAPTCHA Image" style="border: 1px solid #000;">
	        <button type="button" onclick="reloadCaptcha()">Reload CAPTCHA</button>
	        <button type="submit">Login</button>
	    </form>
	</body>
	</html>`)
	if err != nil {
		http.Error(w, "Could not parse login template", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, timestamp)
}

// Serve the CAPTCHA image
func serveCaptchaImage(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(captchaPath)
	if err != nil {
		log.Printf("Could not open CAPTCHA image: %v", err)
		http.Error(w, "Could not open CAPTCHA image", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Could not get CAPTCHA file info: %v", err)
		http.Error(w, "Could not get CAPTCHA file info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "captcha.jpg", fileInfo.ModTime(), file)
}

// Handle CAPTCHA reload
func reloadCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Reload the SRM login page
	_, err := srmPage.Goto("https://sp.srmist.edu.in/srmiststudentportal/students/loginManager/youLogin.jsp", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		http.Error(w, "Failed to reload SRM page", http.StatusInternalServerError)
		return
	}

	// Capture the new CAPTCHA
	captureCaptcha()
	w.WriteHeader(http.StatusOK)
}

// Handle login form submission
func submitLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	netid := r.FormValue("netid")
	password := r.FormValue("password")
	captcha := r.FormValue("captcha")

	err := srmPage.Fill("#login", netid)
	if err != nil {
		http.Error(w, "Could not fill NetID", http.StatusInternalServerError)
		return
	}

	err = srmPage.Fill("#passwd", password)
	if err != nil {
		http.Error(w, "Could not fill password", http.StatusInternalServerError)
		return
	}

	err = srmPage.Fill("#ccode", captcha)
	if err != nil {
		http.Error(w, "Could not fill CAPTCHA", http.StatusInternalServerError)
		return
	}

	err = srmPage.Click("button.btn-custom.btn-user.btn-block.lift")
	if err != nil {
		http.Error(w, "Could not click login button", http.StatusInternalServerError)
		return
	}

	time.Sleep(3 * time.Second)

	// Navigate to the attendance page
	err = srmPage.Click("#listId9")
	if err != nil {
		http.Error(w, "Could not click Attendance Details link", http.StatusInternalServerError)
		return
	}

	time.Sleep(3 * time.Second)

	attendanceTableElement, err := srmPage.QuerySelector(".table-responsive.table-billing-history > table")
	if err != nil {
		http.Error(w, "Could not query attendance table", http.StatusInternalServerError)
		return
	}
	if attendanceTableElement == nil {
		http.Error(w, "Attendance table not found", http.StatusInternalServerError)
		return
	}

	attendanceRows, err := attendanceTableElement.QuerySelectorAll("tbody tr")
	if err != nil {
		http.Error(w, "Could not query attendance rows", http.StatusInternalServerError)
		return
	}

	attendanceData := ""
	for _, row := range attendanceRows {
		columns, err := row.QuerySelectorAll("td")
		if err != nil {
			http.Error(w, "Could not query columns in attendance row", http.StatusInternalServerError)
			return
		}
		if len(columns) == 8 {
			maxHours, _ := columns[2].InnerText()
			attHours, _ := columns[3].InnerText()
			var att, tot int
			fmt.Sscanf(attHours, "%d", &att)
			fmt.Sscanf(maxHours, "%d", &tot)
			margin := calculateMargin(att, tot)
			rowHTML, _ := row.InnerHTML()
			attendanceData += fmt.Sprintf("<tr>%s<td>%d <a href='#' onclick=\"alert('%s')\"><i class='fa fa-info-circle'></i></a></td></tr>", rowHTML, margin.Hours, margin.Message)
		}
	}

	tmpl, err := template.New("attendance").Parse(`<!DOCTYPE html>
	<html lang="en">
	<head>
	    <meta charset="UTF-8">
	    <title>Attendance Details</title>
	    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/bootstrap/5.1.3/css/bootstrap.min.css">
	    <script src="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/js/all.min.js"></script>
	</head>
	<body>
	    <div class="container mt-5">
	        <h2>Attendance Details</h2>
	        <div class="table-responsive table-billing-history">
	            <table class="table table-bordered">
	                <thead class="table-light">
	                    <tr>
	                        <th scope="col">Code</th>
	                        <th scope="col">Description</th>
	                        <th scope="col">Max. hours</th>
	                        <th scope="col">Att. hours</th>
	                        <th scope="col">Absent hours</th>
	                        <th scope="col">Average %</th>
	                        <th scope="col">OD/ML Percentage</th>
	                        <th scope="col">Total Percentage</th>
	                        <th scope="col">Margin</th>
	                    </tr>
	                </thead>
	                <tbody>
	                    {{.}}
	                </tbody>
	            </table>
	        </div>
	    </div>
	</body>
	</html>`)
	if err != nil {
		http.Error(w, "Could not parse attendance template", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, template.HTML(attendanceData))
}

// Margin calculation
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
				return Margin{
					Hours: n - 1,
					Message: fmt.Sprintf(
						"You can bunk %d more hours to get to %d / %d = %.2f%%",
						n-1,
						att,
						tot+(n-1),
						(float32(att) / float32(tot+(n-1)) * 100),
					),
				}
			}
			n++
		}
	} else if initAtp >= 75 && initAtp < 76 {
		return Margin{
			Hours:   0,
			Message: fmt.Sprintf("Your attendance is already between 75%% and 76%% at %.2f%%. Maintain this level to stay above 75%%.", initAtp),
		}
	} else {
		n := 0
		for {
			atp := (float32(att+n) / float32(tot+n)) * 100
			if atp >= 75 {
				return Margin{
					Hours:   n,
					Message: fmt.Sprintf("You need to attend %d more hours to get to %d / %d = %.2f%%", n, att+n, tot+n, atp),
				}
			}
			n++
		}
	}
}
