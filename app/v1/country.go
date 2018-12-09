package v1
import (
	"zxq.co/ripple/rippleapi/common"
)

type Country struct {
	Name	string 	`json:"name"`
	Code	string	`json:"code"`
}

type MultiCount struct {
	common.ResponseBase
	Countries	[]Country	`json:"countries"`
}
func CountriesGET(md common.MethodData) common.CodeMessager {
	var r MultiCount
	
	r.Countries = []Country{
				{
			Name: "Andorra",
			Code:  "AD",
		},
		{
			Name: "United Arab Emirates",
			Code:  "AE",
		},
		{
			Name: "Afghanistan",
			Code:  "AF",
		},
		{
			Name: "Antigua and Barbuda",
			Code:  "AG",
		},
		{
			Name: "Anguilla",
			Code:  "AI",
		},
		{
			Name: "Albania",
			Code:  "AL",
		},
		{
			Name: "Armenia",
			Code:  "AM",
		},
		{
			Name: "Angola",
			Code:  "AO",
		},
		{
			Name: "Antarctica",
			Code:  "AQ",
		},
		{
			Name: "Argentina",
			Code:  "AR",
		},
		{
			Name: "American Samoa",
			Code:  "AS",
		},
		{
			Name: "Austria",
			Code:  "AT",
		},
		{
			Name: "Australia",
			Code:  "AU",
		},
		{
			Name: "Aruba",
			Code:  "AW",
		},
		{
			Name: "Aland Islands",
			Code:  "AX",
		},
		{
			Name: "Azerbaijan",
			Code:  "AZ",
		},
		{
			Name: "Bosnia and Herzegovina",
			Code:  "BA",
		},
		{
			Name: "Barbados",
			Code:  "BB",
		},
		{
			Name: "Belgium",
			Code:  "BE",
		},
		{
			Name: "Burkina Faso",
			Code:  "BF",
		},
		{
			Name: "Bulgaria",
			Code:  "BG",
		},
		{
			Name: "Bahrain",
			Code:  "BH",
		},
		{
			Name: "Burundi",
			Code:  "BI",
		},
		{
			Name: "Benin",
			Code:  "BJ",
		},
		{
			Name: "Saint Barthelemy",
			Code:  "BL",
		},
		{
			Name: "Bermuda",
			Code:  "BM",
		},
		{
			Name: "Brunei",
			Code:  "BN",
		},
		{
			Name: "Bolivia",
			Code:  "BO",
		},
		{
			Name: "Saint Eustatius and Saba Bonaire",
			Code:  "BQ",
		},
		{
			Name: "Brazil",
			Code:  "BR",
		},
		{
			Name: "Bahamas",
			Code:  "BS",
		},
		{
			Name: "Bhutan",
			Code:  "BT",
		},
		{
			Name: "Bouvet Island",
			Code:  "BV",
		},
		{
			Name: "Botswana",
			Code:  "BW",
		},
		{
			Name: "Belarus",
			Code:  "BY",
		},
		{
			Name: "Belize",
			Code:  "BZ",
		},
		{
			Name: "Canada",
			Code:  "CA",
		},
		{
			Name: "Cocos Islands",
			Code:  "CC",
		},
		{
			Name: "Democratic Republic of the Congo",
			Code:  "CD",
		},
		{
			Name: "Central African Republic",
			Code:  "CF",
		},
		{
			Name: "Republic of the Congo",
			Code:  "CG",
		},
		{
			Name: "Switzerland",
			Code:  "CH",
		},
		{
			Name: "Ivory Coast",
			Code:  "CI",
		},
		{
			Name: "Cook Islands",
			Code:  "CK",
		},
		{
			Name: "Chile",
			Code:  "CL",
		},
		{
			Name: "Cameroon",
			Code:  "CM",
		},
		{
			Name: "China",
			Code:  "CN",
		},
		{
			Name: "Colombia",
			Code:  "CO",
		},
		{
			Name: "Costa Rica",
			Code:  "CR",
		},
		{
			Name: "Cuba",
			Code:  "CU",
		},
		{
			Name: "Cape Verde",
			Code:  "CV",
		},
		{
			Name: "Curacao",
			Code:  "CW",
		},
		{
			Name: "Christmas Island",
			Code:  "CX",
		},
		{
			Name: "Cyprus",
			Code:  "CY",
		},
		{
			Name: "Czech Republic",
			Code:  "CZ",
		},
		{
			Name: "Germany",
			Code:  "DE",
		},
		{
			Name: "Djibouti",
			Code:  "DJ",
		},
		{
			Name: "Denmark",
			Code:  "DK",
		},
		{
			Name: "Dominica",
			Code:  "DM",
		},
		{
			Name: "Dominican Republic",
			Code:  "DO",
		},
		{
			Name: "Algeria",
			Code:  "DZ",
		},
		{
			Name: "Ecuador",
			Code:  "EC",
		},
		{
			Name: "Estonia",
			Code:  "EE",
		},
		{
			Name: "Egypt",
			Code:  "EG",
		},
		{
			Name: "Western Sahara",
			Code:  "EH",
		},
		{
			Name: "Eritrea",
			Code:  "ER",
		},
		{
			Name: "Spain",
			Code:  "ES",
		},
		{
			Name: "Ethiopia",
			Code:  "ET",
		},
		{
			Name: "Finland",
			Code:  "FI",
		},
		{
			Name: "Fiji",
			Code:  "FJ",
		},
		{
			Name: "Falkland Islands",
			Code:  "FK",
		},
		{
			Name: "Micronesia",
			Code:  "FM",
		},
		{
			Name: "Faroe Islands",
			Code:  "FO",
		},
		{
			Name: "France",
			Code:  "FR",
		},
		{
			Name: "Gabon",
			Code:  "GA",
		},
		{
			Name: "United Kingdom",
			Code:  "GB",
		},
		{
			Name: "Grenada",
			Code:  "GD",
		},
		{
			Name: "Georgia",
			Code:  "GE",
		},
		{
			Name: "French Guiana",
			Code:  "GF",
		},
		{
			Name: "Guernsey",
			Code:  "GG",
		},
		{
			Name: "Ghana",
			Code:  "GH",
		},
		{
			Name: "Gibraltar",
			Code:  "GI",
		},
		{
			Name: "Greenland",
			Code:  "GL",
		},
		{
			Name: "Gambia",
			Code:  "GM",
		},
		{
			Name: "Guinea",
			Code:  "GN",
		},
		{
			Name: "Guadeloupe",
			Code:  "GP",
		},
		{
			Name: "Equatorial Guinea",
			Code:  "GQ",
		},
		{
			Name: "Greece",
			Code:  "GR",
		},
		{
			Name: "South Georgia and the South Sandwich Islands",
			Code:  "GS",
		},
		{
			Name: "Guatemala",
			Code:  "GT",
		},
		{
			Name: "Guam",
			Code:  "GU",
		},
		{
			Name: "Guinea-Bissau",
			Code:  "GW",
		},
		{
			Name: "Guyana",
			Code:  "GY",
		},
		{
			Name: "Hong Kong",
			Code:  "HK",
		},
		{
			Name: "Heard Island and McDonald Islands",
			Code:  "HM",
		},
		{
			Name: "Honduras",
			Code:  "HN",
		},
		{
			Name: "Croatia",
			Code:  "HR",
		},
		{
			Name: "Haiti",
			Code:  "HT",
		},
		{
			Name: "Hungary",
			Code:  "HU",
		},
		{
			Name: "Indonesia",
			Code:  "ID",
		},
		{
			Name: "Ireland",
			Code:  "IE",
		},
		{
			Name: "Israel",
			Code:  "IL",
		},
		{
			Name: "Isle of Man",
			Code:  "IM",
		},
		{
			Name: "India",
			Code:  "IN",
		},
		{
			Name: "British Indian Ocean Territory",
			Code:  "IO",
		},
		{
			Name: "Iraq",
			Code:  "IQ",
		},
		{
			Name: "Iran",
			Code:  "IR",
		},
		{
			Name: "Iceland",
			Code:  "IS",
		},
		{
			Name: "Italy",
			Code:  "IT",
		},
		{
			Name: "Jersey",
			Code:  "JE",
		},
		{
			Name: "Jamaica",
			Code:  "JM",
		},
		{
			Name: "Jordan",
			Code:  "JO",
		},
		{
			Name: "Japan",
			Code:  "JP",
		},
		{
			Name: "Kenya",
			Code:  "KE",
		},
		{
			Name: "Kyrgyzstan",
			Code:  "KG",
		},
		{
			Name: "Cambodia",
			Code:  "KH",
		},
		{
			Name: "Kiribati",
			Code:  "KI",
		},
		{
			Name: "Comoros",
			Code:  "KM",
		},
		{
			Name: "Saint Kitts and Nevis",
			Code:  "KN",
		},
		{
			Name: "North Korea",
			Code:  "KP",
		},
		{
			Name: "South Korea",
			Code:  "KR",
		},
		{
			Name: "Kuwait",
			Code:  "KW",
		},
		{
			Name: "Cayman Islands",
			Code:  "KY",
		},
		{
			Name: "Kazakhstan",
			Code:  "KZ",
		},
		{
			Name: "Laos",
			Code:  "LA",
		},
		{
			Name: "Lebanon",
			Code:  "LB",
		},
		{
			Name: "Saint Lucia",
			Code:  "LC",
		},
		{
			Name: "Liechtenstein",
			Code:  "LI",
		},
		{
			Name: "Sri Lanka",
			Code:  "LK",
		},
		{
			Name: "Liberia",
			Code:  "LR",
		},
		{
			Name: "Lesotho",
			Code:  "LS",
		},
		{
			Name: "Lithuania",
			Code:  "LT",
		},
		{
			Name: "Luxembourg",
			Code:  "LU",
		},
		{
			Name: "Latvia",
			Code:  "LV",
		},
		{
			Name: "Libya",
			Code:  "LY",
		},
		{
			Name: "Morocco",
			Code:  "MA",
		},
		{
			Name: "Monaco",
			Code:  "MC",
		},
		{
			Name: "Moldova",
			Code:  "MD",
		},
		{
			Name: "Montenegro",
			Code:  "ME",
		},
		{
			Name: "Saint Martin",
			Code:  "MF",
		},
		{
			Name: "Madagascar",
			Code:  "MG",
		},
		{
			Name: "Marshall Islands",
			Code:  "MH",
		},
		{
			Name: "Macedonia",
			Code:  "MK",
		},
		{
			Name: "Mali",
			Code:  "ML",
		},
		{
			Name: "Myanmar",
			Code:  "MM",
		},
		{
			Name: "Mongolia",
			Code:  "MN",
		},
		{
			Name: "Macao",
			Code:  "MO",
		},
		{
			Name: "Northern Mariana Islands",
			Code:  "MP",
		},
		{
			Name: "Martinique",
			Code:  "MQ",
		},
		{
			Name: "Mauritania",
			Code:  "MR",
		},
		{
			Name: "Montserrat",
			Code:  "MS",
		},
		{
			Name: "Malta",
			Code:  "MT",
		},
		{
			Name: "Mauritius",
			Code:  "MU",
		},
		{
			Name: "Maldives",
			Code:  "MV",
		},
		{
			Name: "Malawi",
			Code:  "MW",
		},
		{
			Name: "Mexico",
			Code:  "MX",
		},
		{
			Name: "Malaysia",
			Code:  "MY",
		},
		{
			Name: "Mozambique",
			Code:  "MZ",
		},
		{
			Name: "Namibia",
			Code:  "NA",
		},
		{
			Name: "New Caledonia",
			Code:  "NC",
		},
		{
			Name: "Niger",
			Code:  "NE",
		},
		{
			Name: "Norfolk Island",
			Code:  "NF",
		},
		{
			Name: "Nigeria",
			Code:  "NG",
		},
		{
			Name: "Nicaragua",
			Code:  "NI",
		},
		{
			Name: "Netherlands",
			Code:  "NL",
		},
		{
			Name: "Norway",
			Code:  "NO",
		},
		{
			Name: "Nepal",
			Code:  "NP",
		},
		{
			Name: "Nauru",
			Code:  "NR",
		},
		{
			Name: "Niue",
			Code:  "NU",
		},
		{
			Name: "New Zealand",
			Code:  "NZ",
		},
		{
			Name: "Oman",
			Code:  "OM",
		},
		{
			Name: "Panama",
			Code:  "PA",
		},
		{
			Name: "Peru",
			Code:  "PE",
		},
		{
			Name: "French Polynesia",
			Code:  "PF",
		},
		{
			Name: "Papua New Guinea",
			Code:  "PG",
		},
		{
			Name: "Philippines",
			Code:  "PH",
		},
		{
			Name: "Pakistan",
			Code:  "PK",
		},
		{
			Name: "Poland",
			Code:  "PL",
		},
		{
			Name: "Saint Pierre and Miquelon",
			Code:  "PM",
		},
		{
			Name: "Pitcairn",
			Code:  "PN",
		},
		{
			Name: "Puerto Rico",
			Code:  "PR",
		},
		{
			Name: "Palestinian Territory",
			Code:  "PS",
		},
		{
			Name: "Portugal",
			Code:  "PT",
		},
		{
			Name: "Palau",
			Code:  "PW",
		},
		{
			Name: "Paraguay",
			Code:  "PY",
		},
		{
			Name: "Qatar",
			Code:  "QA",
		},
		{
			Name: "Reunion",
			Code:  "RE",
		},
		{
			Name: "Romania",
			Code:  "RO",
		},
		{
			Name: "Serbia",
			Code:  "RS",
		},
		{
			Name: "Russia",
			Code:  "RU",
		},
		{
			Name: "Rwanda",
			Code:  "RW",
		},
		{
			Name: "Saudi Arabia",
			Code:  "SA",
		},
		{
			Name: "Solomon Islands",
			Code:  "SB",
		},
		{
			Name: "Seychelles",
			Code:  "SC",
		},
		{
			Name: "Sudan",
			Code:  "SD",
		},
		{
			Name: "Sweden",
			Code:  "SE",
		},
		{
			Name: "Singapore",
			Code:  "SG",
		},
		{
			Name: "Saint Helena",
			Code:  "SH",
		},
		{
			Name: "Slovenia",
			Code:  "SI",
		},
		{
			Name: "Svalbard and Jan Mayen",
			Code:  "SJ",
		},
		{
			Name: "Slovakia",
			Code:  "SK",
		},
		{
			Name: "Sierra Leone",
			Code:  "SL",
		},
		{
			Name: "San Marino",
			Code:  "SM",
		},
		{
			Name: "Senegal",
			Code:  "SN",
		},
		{
			Name: "Somalia",
			Code:  "SO",
		},
		{
			Name: "Suriname",
			Code:  "SR",
		},
		{
			Name: "South Sudan",
			Code:  "SS",
		},
		{
			Name: "Sao Tome and Principe",
			Code:  "ST",
		},
		{
			Name: "El Salvador",
			Code:  "SV",
		},
		{
			Name: "Sint Maarten",
			Code:  "SX",
		},
		{
			Name: "Syria",
			Code:  "SY",
		},
		{
			Name: "Swaziland",
			Code:  "SZ",
		},
		{
			Name: "Turks and Caicos Islands",
			Code:  "TC",
		},
		{
			Name: "Chad",
			Code:  "TD",
		},
		{
			Name: "French Southern Territories",
			Code:  "TF",
		},
		{
			Name: "Togo",
			Code:  "TG",
		},
		{
			Name: "Thailand",
			Code:  "TH",
		},
		{
			Name: "Tajikistan",
			Code:  "TJ",
		},
		{
			Name: "Tokelau",
			Code:  "TK",
		},
		{
			Name: "East Timor",
			Code:  "TL",
		},
		{
			Name: "Turkmenistan",
			Code:  "TM",
		},
		{
			Name: "Tunisia",
			Code:  "TN",
		},
		{
			Name: "Tonga",
			Code:  "TO",
		},
		{
			Name: "Turkey",
			Code:  "TR",
		},
		{
			Name: "Trinidad and Tobago",
			Code:  "TT",
		},
		{
			Name: "Tuvalu",
			Code:  "TV",
		},
		{
			Name: "Taiwan",
			Code:  "TW",
		},
		{
			Name: "Tanzania",
			Code:  "TZ",
		},
		{
			Name: "Ukraine",
			Code:  "UA",
		},
		{
			Name: "Uganda",
			Code:  "UG",
		},
		{
			Name: "United States Minor Outlying Islands",
			Code:  "UM",
		},
		{
			Name: "United States",
			Code:  "US",
		},
		{
			Name: "Uruguay",
			Code:  "UY",
		},
		{
			Name: "Uzbekistan",
			Code:  "UZ",
		},
		{
			Name: "Vatican",
			Code:  "VA",
		},
		{
			Name: "Saint Vincent and the Grenadines",
			Code:  "VC",
		},
		{
			Name: "Venezuela",
			Code:  "VE",
		},
		{
			Name: "British Virgin Islands",
			Code:  "VG",
		},
		{
			Name: "U.S. Virgin Islands",
			Code:  "VI",
		},
		{
			Name: "Vietnam",
			Code:  "VN",
		},
		{
			Name: "Vanuatu",
			Code:  "VU",
		},
		{
			Name: "Wallis and Futuna",
			Code:  "WF",
		},
		{
			Name: "Samoa",
			Code:  "WS",
		},
		{
			Name: "Kosovo",
			Code:  "XK",
		},
		{
			Name: "Yemen",
			Code:  "YE",
		},
		{
			Name: "Mayotte",
			Code:  "YT",
		},
		{
			Name: "South Africa",
			Code:  "ZA",
		},
		{
			Name: "Zambia",
			Code:  "ZM",
		},
		{
			Name: "Zimbabwe",
			Code:  "ZW",
		},
		{
			Name: "Bangladesh",
			Code: "BD",
		},
		{
			Name:  "Unknown country",
			Code: "XX",
		},
	}
	
	r.Code = 200
	return r
}