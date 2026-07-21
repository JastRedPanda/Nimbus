package tray

import "fmt"

type cfgShim struct {
	CityName     string
	Latitude     float64
	Longitude    float64
	Units        string
	PressureUnit string
	IconTheme    string
	Language     string
}

func settingsScript(cfg *cfgShim, path string) string {
	units := cfg.Units
	if units == "" {
		units = "celsius"
	}
	pres := cfg.PressureUnit
	if pres == "" {
		pres = "hpa"
	}
	theme := cfg.IconTheme
	if theme == "" {
		theme = "auto"
	}
	lang := cfg.Language
	if lang == "" {
		lang = "en"
	}

	return fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms,System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = "Nimbus Settings"
$form.Size = New-Object System.Drawing.Size(440,480)
$form.StartPosition = "CenterScreen"
$form.FormBorderStyle = "FixedDialog"
$form.MaximizeBox = $false
$form.MinimizeBox = $false

$y = 10

# ---------- City ----------
$lbl = New-Object System.Windows.Forms.Label
$lbl.Text = "City:"
$lbl.Location = New-Object System.Drawing.Point(10,$y)
$lbl.Size = New-Object System.Drawing.Size(80,22)
$form.Controls.Add($lbl)

$txtCity = New-Object System.Windows.Forms.TextBox
$txtCity.Location = New-Object System.Drawing.Point(100,$y)
$txtCity.Size = New-Object System.Drawing.Size(200,22)
$txtCity.Text = "%s"
$form.Controls.Add($txtCity)

$btnSearch = New-Object System.Windows.Forms.Button
$btnSearch.Text = "Search"
$btnSearch.Location = New-Object System.Drawing.Point(310,$y-1)
$btnSearch.Size = New-Object System.Drawing.Size(70,24)
$form.Controls.Add($btnSearch)

$lbCities = New-Object System.Windows.Forms.ListBox
$lbCities.Location = New-Object System.Drawing.Point(100,$y+26)
$lbCities.Size = New-Object System.Drawing.Size(280,80)
$lbCities.Visible = $false
$form.Controls.Add($lbCities)

$btnSearch.Add_Click({
	$q = $txtCity.Text.Trim()
	if ($q -eq "") { return }
	try {
		$url = "https://geocoding-api.open-meteo.com/v1/search?name=" + [System.Web.HttpUtility]::UrlEncode($q) + "&count=10&language=en&format=json"
		$resp = Invoke-RestMethod -Uri $url -TimeoutSec 5
		$lbCities.Items.Clear()
		if ($resp.results) {
			$resp.results | ForEach-Object {
				$lbCities.Items.Add($_.name + ", " + $_.country + " | " + $_.latitude + ", " + $_.longitude)
			}
			$lbCities.Visible = $true
		}
	} catch { }
})

$lbCities.Add_SelectedIndexChanged({
	if ($lbCities.SelectedItem -ne $null) {
		$parts = $lbCities.SelectedItem -split " \| "
		$coords = $parts[1] -split ", "
		$txtCity.Text = $parts[0]
		$txtLat.Text = $coords[0]
		$txtLon.Text = $coords[1]
		$lbCities.Visible = $false
	}
})

# ---------- Latitude ----------
$y += 60

$lblLat = New-Object System.Windows.Forms.Label
$lblLat.Text = "Latitude:"
$lblLat.Location = New-Object System.Drawing.Point(10,$y)
$lblLat.Size = New-Object System.Drawing.Size(80,22)
$form.Controls.Add($lblLat)

$txtLat = New-Object System.Windows.Forms.TextBox
$txtLat.Location = New-Object System.Drawing.Point(100,$y)
$txtLat.Size = New-Object System.Drawing.Size(100,22)
$txtLat.Text = "%.4f"
$form.Controls.Add($txtLat)

# ---------- Longitude ----------
$y += 30

$lblLon = New-Object System.Windows.Forms.Label
$lblLon.Text = "Longitude:"
$lblLon.Location = New-Object System.Drawing.Point(10,$y)
$lblLon.Size = New-Object System.Drawing.Size(80,22)
$form.Controls.Add($lblLon)

$txtLon = New-Object System.Windows.Forms.TextBox
$txtLon.Location = New-Object System.Drawing.Point(100,$y)
$txtLon.Size = New-Object System.Drawing.Size(100,22)
$txtLon.Text = "%.4f"
$form.Controls.Add($txtLon)

# ---------- Temperature ----------
$y += 40

$grpT = New-Object System.Windows.Forms.GroupBox
$grpT.Text = "Temperature"
$grpT.Location = New-Object System.Drawing.Point(10,$y)
$grpT.Size = New-Object System.Drawing.Size(180,48)
$form.Controls.Add($grpT)

$rbC = New-Object System.Windows.Forms.RadioButton
$rbC.Text = "°C"
$rbC.Location = New-Object System.Drawing.Point(10,18)
$rbC.Size = New-Object System.Drawing.Size(60,22)
if ("%s" -eq "celsius") { $rbC.Checked = $true }
$grpT.Controls.Add($rbC)

$rbF = New-Object System.Windows.Forms.RadioButton
$rbF.Text = "°F"
$rbF.Location = New-Object System.Drawing.Point(90,18)
$rbF.Size = New-Object System.Drawing.Size(60,22)
if ("%s" -eq "fahrenheit") { $rbF.Checked = $true }
$grpT.Controls.Add($rbF)

# ---------- Pressure ----------
$y += 58

$grpP = New-Object System.Windows.Forms.GroupBox
$grpP.Text = "Pressure"
$grpP.Location = New-Object System.Drawing.Point(10,$y)
$grpP.Size = New-Object System.Drawing.Size(320,48)
$form.Controls.Add($grpP)

$rbH = New-Object System.Windows.Forms.RadioButton
$rbH.Text = "hPa"
$rbH.Location = New-Object System.Drawing.Point(10,18)
$rbH.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "hpa") { $rbH.Checked = $true }
$grpP.Controls.Add($rbH)

$rbM = New-Object System.Windows.Forms.RadioButton
$rbM.Text = "mmHg"
$rbM.Location = New-Object System.Drawing.Point(90,18)
$rbM.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "mmhg") { $rbM.Checked = $true }
$grpP.Controls.Add($rbM)

$rbI = New-Object System.Windows.Forms.RadioButton
$rbI.Text = "inHg"
$rbI.Location = New-Object System.Drawing.Point(170,18)
$rbI.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "inhg") { $rbI.Checked = $true }
$grpP.Controls.Add($rbI)

# ---------- Icon Theme ----------
$y += 58

$grpTh = New-Object System.Windows.Forms.GroupBox
$grpTh.Text = "Icon Theme"
$grpTh.Location = New-Object System.Drawing.Point(10,$y)
$grpTh.Size = New-Object System.Drawing.Size(320,48)
$form.Controls.Add($grpTh)

$thA = New-Object System.Windows.Forms.RadioButton
$thA.Text = "Auto"
$thA.Location = New-Object System.Drawing.Point(10,18)
$thA.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "auto") { $thA.Checked = $true }
$grpTh.Controls.Add($thA)

$thD = New-Object System.Windows.Forms.RadioButton
$thD.Text = "Dark"
$thD.Location = New-Object System.Drawing.Point(90,18)
$thD.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "dark") { $thD.Checked = $true }
$grpTh.Controls.Add($thD)

$thL = New-Object System.Windows.Forms.RadioButton
$thL.Text = "Light"
$thL.Location = New-Object System.Drawing.Point(170,18)
$thL.Size = New-Object System.Drawing.Size(70,22)
if ("%s" -eq "light") { $thL.Checked = $true }
$grpTh.Controls.Add($thL)

# ---------- Language ----------
$y += 58

$grpL = New-Object System.Windows.Forms.GroupBox
$grpL.Text = "Language"
$grpL.Location = New-Object System.Drawing.Point(10,$y)
$grpL.Size = New-Object System.Drawing.Size(200,48)
$form.Controls.Add($grpL)

$rbEn = New-Object System.Windows.Forms.RadioButton
$rbEn.Text = "English"
$rbEn.Location = New-Object System.Drawing.Point(10,18)
$rbEn.Size = New-Object System.Drawing.Size(80,22)
if ("%s" -eq "en") { $rbEn.Checked = $true }
$grpL.Controls.Add($rbEn)

$rbUk = New-Object System.Windows.Forms.RadioButton
$rbUk.Text = "Українська"
$rbUk.Location = New-Object System.Drawing.Point(100,18)
$rbUk.Size = New-Object System.Drawing.Size(90,22)
if ("%s" -eq "uk") { $rbUk.Checked = $true }
$grpL.Controls.Add($rbUk)

# ---------- Buttons ----------
$y += 68

$btnOk = New-Object System.Windows.Forms.Button
$btnOk.Text = "Save"
$btnOk.Location = New-Object System.Drawing.Point(110,$y)
$btnOk.Size = New-Object System.Drawing.Size(90,30)
$btnOk.DialogResult = "OK"
$form.Controls.Add($btnOk)

$btnCancel = New-Object System.Windows.Forms.Button
$btnCancel.Text = "Cancel"
$btnCancel.Location = New-Object System.Drawing.Point(220,$y)
$btnCancel.Size = New-Object System.Drawing.Size(90,30)
$btnCancel.DialogResult = "Cancel"
$form.Controls.Add($btnCancel)

$result = $form.ShowDialog()
if ($result -eq "OK") {
    $unit = "celsius"
    if ($rbF.Checked) { $unit = "fahrenheit" }
    $pres = "hpa"
    if ($rbM.Checked) { $pres = "mmhg" }
    if ($rbI.Checked) { $pres = "inhg" }
    $th = "auto"
    if ($thD.Checked) { $th = "dark" }
    if ($thL.Checked) { $th = "light" }
    $lang = "en"
    if ($rbUk.Checked) { $lang = "uk" }
    $lat = [double]::Parse($txtLat.Text)
    $lon = [double]::Parse($txtLon.Text)
    $city = $txtCity.Text
    @"
{"latitude":$lat,"longitude":$lon,"city_name":"$city","update_interval":10,"units":"$unit","pressure_unit":"$pres","icon_theme":"$th","language":"$lang"}
"@
}
`, cfg.CityName, cfg.Latitude, cfg.Longitude, units, units, pres, pres, pres, theme, theme, theme, lang, lang)
}


