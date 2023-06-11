package converter

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"github.com/signintech/gopdf"
	"github.com/skip2/go-qrcode"
	"github.com/unidoc/unioffice/common/license"
	"github.com/unidoc/unioffice/document"
	"github.com/unidoc/unioffice/document/convert"
	unipdflicense "github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/creator"
	"github.com/unidoc/unipdf/v3/model"
	"github.com/unidoc/unipdf/v3/model/xmputil"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMargin   = 5.0
	rightCorner     = 520
	bottomCorner    = 770
	apiKey          = "f5ea7467200af934b07f052b0b51732e795a463df986dc2f6fd9d3d9df36bb0c"
	privyImg        = "qr_bg.png"
	privyResizedImg = "qr_bg_resized.png"
)

type PosQr struct {
	X      float64
	Y      float64
	NameQr string
}

type Stamp struct {
	TopLeft     PosQr
	TopRight    PosQr
	BottomLeft  PosQr
	BottomRight PosQr
	CustomPos   *PosQr
}

type Option func(*Options)

type Options struct {
	X           *float64
	Y           *float64
	PDFPassword *string
}

func NewProcessFiles(inputPath string, options ...Option) error {
	// Membuat variabel default untuk opsi
	opt := &Options{
		X:           nil,
		Y:           nil,
		PDFPassword: nil,
	}
	
	// Menjalankan opsi-opsi yang diberikan
	for _, optFunc := range options {
		optFunc(opt)
	}
	
	_, err := os.Stat("output")
	if os.IsNotExist(err) {
		// "output" directory does not exist, create it
		err := os.Mkdir("output", os.ModePerm)
		if err != nil {
			return err
		}
	} else if err != nil {
		// Error occurred while checking the directory
		return err
	}
	
	_, err = os.Stat("img")
	if os.IsNotExist(err) {
		// "output" directory does not exist, create it
		err := os.Mkdir("img", os.ModePerm)
		if err != nil {
			return err
		}
	} else if err != nil {
		// Error occurred while checking the directory
		return err
	}
	
	files, err := os.ReadDir(inputPath)
	if err != nil {
		return err
	}
	
	for _, file := range files {
		if !file.IsDir() {
			filePath := filepath.Join(inputPath, file.Name())
			extension := strings.ToLower(filepath.Ext(filePath))
			switch extension {
			case ".pdf", ".jpg", ".jpeg", ".png":
				err := processFile(filePath, extension, opt)
				if err != nil {
					log.Printf("Error processing file %s: %s", filePath, err)
				}
			case ".docx":
				initLicense()
				outputPath := filepath.Join("output", filepath.Base(filePath))
				pdfPath := strings.TrimSuffix(outputPath, extension) + ".pdf"
				err := convertDocxToPdf(filePath, pdfPath)
				if err != nil {
					log.Printf("Error converting DOCX file %s to PDF: %s", filePath, err)
				} else {
					// Add QR code to the new PDF
					err = addQRCodeToPdf(pdfPath, opt)
					if err != nil {
						log.Printf("Error processing converted PDF file %s: %s", pdfPath, err)
					}
				}
			default:
				log.Printf("Unsupported file format: %s", extension)
			}
			fmt.Println("Processed file:", filePath)
		}
	}
	
	err = deleteFilesInFolder("img")
	if err != nil {
		return err
	}
	
	return nil
}

func WithX(x float64) Option {
	return func(opt *Options) {
		opt.X = &x
	}
}

func WithY(y float64) Option {
	return func(opt *Options) {
		opt.Y = &y
	}
}

func WithPDFPassword(password string) Option {
	return func(opt *Options) {
		opt.PDFPassword = &password
	}
}

func processFile(filePath string, extension string, opt *Options) error {
	outputPath := filepath.Join("output", filepath.Base(filePath))
	switch extension {
	case ".pdf":
		isPwd, err := isPdfPasswordProtected(filePath)
		if err != nil {
			return err
		}
		
		if isPwd {
			initLicense()
			err = processPasswordProtectedPdf(filePath, opt, outputPath)
			if err != nil {
				return err
			}
		} else {
			// Copy existing PDF content to new PDF
			err = copyPdfContent(filePath, outputPath)
			if err != nil {
				return err
			}
			// Add QR code to the new PDF
			err = addQRCodeToPdf(outputPath, opt)
			if err != nil {
				return err
			}
		}
	
	case ".jpg", ".jpeg", ".png":
		//err := resizeImage(filePath, outImgResized, uint(75), uint(75))
		//if err != nil {
		//	return err
		//}
		pdfPath := strings.TrimSuffix(outputPath, extension) + ".pdf"
		err := convertImageToPdf(filePath, pdfPath)
		if err != nil {
			return err
		}
		
		// Add QR code to the new PDF
		err = addQRCodeToPdf(pdfPath, opt)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported file format: %s", extension)
	}
	
	log.Printf("Processed file: %s", outputPath)
	return nil
}

func convertImageToPdf(imagePath string, pdfPath string) error {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()
	
	err := pdf.Image(imagePath, 0, 0, nil)
	if err != nil {
		return err
	}
	err = pdf.WritePdf(pdfPath)
	if err != nil {
		return err
	}
	return nil
}

func convertDocxToPdf(docxPath, outPath string) error {
	doc, err := document.Open(docxPath)
	if err != nil {
		return err
	}
	defer doc.Close()
	
	c := convert.ConvertToPdf(doc)
	err = c.WriteToFile(outPath)
	if err != nil {
		log.Fatalf("error converting document: %s", err)
	}
	return nil
}

func generateQR() string {
	// Generate random UUID
	uuid := uuid.New().String()
	qrURL := fmt.Sprintf("https://privy.id/verify/%s", uuid)
	_ = qrcode.WriteFile(qrURL, qrcode.Medium, 125, fmt.Sprintf("img/%s.png", uuid))
	
	return fmt.Sprintf("img/%s.png", uuid)
}

func addQRCodeToPdf(outputPath string, opt *Options) error {
	qrSize := 75.0
	err := resizeImage(privyImg, privyResizedImg, uint(qrSize), uint(qrSize))
	if err != nil {
		return err
	}
	
	a := Stamp{
		TopLeft: PosQr{
			X:      defaultMargin,
			Y:      defaultMargin,
			NameQr: generateQR(),
		},
		TopRight: PosQr{
			X:      rightCorner,
			Y:      defaultMargin,
			NameQr: generateQR(),
		},
		BottomLeft: PosQr{
			X:      defaultMargin,
			Y:      bottomCorner,
			NameQr: generateQR(),
		},
		BottomRight: PosQr{
			X:      rightCorner,
			Y:      bottomCorner,
			NameQr: generateQR(),
		},
	}
	
	if opt.X != nil && opt.Y != nil {
		a.CustomPos = &PosQr{
			X:      *opt.X,
			Y:      *opt.Y,
			NameQr: generateQR(),
		}
		
	}
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	//pdf.AddPage()
	
	// Import page 1
	pageno := 1
	tpls := []int{pdf.ImportPage(outputPath, 1, "/MediaBox")}
	pdf.AddPage()
	pdf.UseImportedTemplate(tpls[0], 0, 0, 595.28, 0)
	
	for {
		pageno++
		// add qrcode
		positions := []PosQr{a.TopLeft, a.TopRight, a.BottomLeft, a.BottomRight, *a.CustomPos}
		for _, pos := range positions {
			err := pdf.Image(pos.NameQr, float64(pos.X), float64(pos.Y), nil)
			if err != nil {
				return err
			}
			
			//err = os.Remove(pos.NameQr)
			//if err != nil {
			//	return err
			//}
			
			// Calculate the center position
			imageWidth := 15.0
			imageHeight := 15.0
			centerX := float64(pos.X) + (qrSize-imageWidth)/2
			centerY := float64(pos.Y) + (qrSize-imageHeight)/2
			
			err = pdf.Image(privyResizedImg, centerX, centerY, &gopdf.Rect{W: imageWidth, H: imageHeight})
			if err != nil {
				return err
			}
		}
		tpl, err := importPdfPage(&pdf, outputPath, pageno, "/MediaBox")
		if err != nil {
			break
		}
		tpls = append(tpls, tpl)
		pdf.AddPage()
		pdf.UseImportedTemplate(tpls[pageno-1], 0, 0, 595.28, 0)
	}
	
	setMetaData(&pdf)
	
	// Remove the resized image file
	err = os.Remove(privyResizedImg)
	if err != nil {
		return err
	}
	
	// Save the PDF with added QR codes
	err = pdf.WritePdf(outputPath)
	if err != nil {
		return err
	}
	
	return nil
}

func resizeImage(inputPath, outputPath string, width, height uint) error {
	//// Get the path of the currently executing file
	//_, filename, _, ok := runtime.Caller(0)
	//if !ok {
	//	return fmt.Errorf("failed to get the path of the currently executing file")
	//}
	//
	//// Resolve the directory path of the currently executing file
	//dir := filepath.Dir(filename)
	//
	//// Construct the absolute input and output paths
	//absInputPath := filepath.Join(dir, inputPath)
	//absOutputPath := filepath.Join(dir, outputPath)
	
	// Open the image file
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Decode the image
	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}
	
	// Resize the image
	resizedImg := resize.Resize(width, height, img, resize.Lanczos3)
	
	// Create the output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	
	// Encode the resized image and save it to the output file
	err = png.Encode(outFile, resizedImg)
	if err != nil {
		return err
	}
	
	return nil
}

func copyPdfContent(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	
	return nil
}

func importPdfPage(pdf *gopdf.GoPdf, filename string, pageno int, box string) (tpl int, err error) {
	defer func() {
		// recover from panic if one occured. Set err to nil otherwise.
		if recover() != nil {
			err = errors.New("array index out of bounds")
		}
	}()
	
	tpl = pdf.ImportPage(filename, pageno, box)
	return
}

func deleteFilesInFolder(folderPath string) error {
	// Baca isi folder
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return err
	}
	
	// Hapus semua file dalam folder
	for _, file := range files {
		err := os.RemoveAll(filepath.Join(folderPath, file.Name()))
		if err != nil {
			return err
		}
	}
	
	return nil
}

func isPdfPasswordProtected(filePath string) (bool, error) {
	
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	
	pdfReader, err := model.NewPdfReader(f)
	if err != nil {
		return false, err
	}
	
	encrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return false, err
	}
	
	return encrypted, nil
}

func processPasswordProtectedPdf(filePath string, opt *Options, outputPath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	
	c := creator.New()
	pdfReader, err := model.NewPdfReader(f)
	//pdfWriter := model.NewPdfWriter()
	
	if err != nil {
		return err
	}
	
	encrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return err
	}
	
	if encrypted {
		_, err := pdfReader.Decrypt([]byte(*opt.PDFPassword))
		if err != nil {
			return err
		}
	}
	
	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return err
	}
	
	qrSize := 75.0
	err = resizeImage(privyImg, privyResizedImg, uint(qrSize), uint(qrSize))
	if err != nil {
		return err
	}
	
	a := Stamp{
		TopLeft: PosQr{
			X:      defaultMargin,
			Y:      defaultMargin,
			NameQr: generateQR(),
		},
		TopRight: PosQr{
			X:      rightCorner,
			Y:      defaultMargin,
			NameQr: generateQR(),
		},
		BottomLeft: PosQr{
			X:      defaultMargin,
			Y:      bottomCorner,
			NameQr: generateQR(),
		},
		BottomRight: PosQr{
			X:      rightCorner,
			Y:      bottomCorner,
			NameQr: generateQR(),
		},
	}
	
	if opt.X != nil && opt.Y != nil {
		a.CustomPos = &PosQr{
			X:      *opt.X,
			Y:      *opt.Y,
			NameQr: generateQR(),
		}
		
	}
	
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return err
		}
		
		err = c.AddPage(page)
		if err != nil {
			return err
		}
		
		positions := []PosQr{a.TopLeft, a.TopRight, a.BottomLeft, a.BottomRight, *a.CustomPos}
		for _, pos := range positions {
			// Prepare the image.
			img, err := c.NewImageFromFile(pos.NameQr)
			if err != nil {
				return err
			}
			img.ScaleToWidth(70)
			img.SetPos(float64(pos.X), float64(pos.Y))
			
			encoder := core.NewDCTEncoder()
			img.SetEncoder(encoder)
			
			imageWidth := 15.0
			imageHeight := 15.0
			centerX := float64(pos.X) + (qrSize-imageWidth)/2
			centerY := float64(pos.Y) + (qrSize-imageHeight)/2
			
			bg, err := c.NewImageFromFile(privyResizedImg)
			if err != nil {
				return err
			}
			
			bg.ScaleToWidth(15)
			bg.SetPos(centerX, centerY)
			bg.SetEncoder(encoder)
			
			c.Draw(img)
			c.Draw(bg)
		}
	}
	
	// Check if the metadata is already defined within given catalog.
	//var xmpDoc *xmputil.Document
	xmpDoc := xmputil.NewDocument()
	
	// Read PdfInfo from the origin file.
	pdfInfo, err := pdfReader.GetPdfInfo()
	if err != nil {
		log.Fatalf("Err: %v", err)
	}
	
	// Set up some custom fields in the document PdfInfo if it doesn't exist.
	var createdAt time.Time
	if pdfInfo.CreationDate != nil {
		createdAt = pdfInfo.CreationDate.ToGoTime()
	} else {
		createdAt = time.Now()
		creationDate, err := model.NewPdfDateFromTime(createdAt)
		if err != nil {
			log.Fatalf("Err: %v", err)
		}
		pdfInfo.CreationDate = &creationDate
	}
	
	modifiedAt := time.Now()
	pdfInfo.Title = core.MakeString("Metadata Baru")
	pdfInfo.Author = core.MakeString("Librantara")
	pdfInfo.Creator = core.MakeString("Librantara")
	pdfInfo.Producer = core.MakeString("MajuTumbuhBersama")
	
	modDate, err := model.NewPdfDateFromTime(modifiedAt)
	if err != nil {
		log.Fatalf("Err: %v", err)
	}
	pdfInfo.ModifiedDate = &modDate
	
	// Copy the content of the PdfInfo into XMP metadata.
	xmpPdfMetadata := &xmputil.PdfInfoOptions{
		InfoDict:   pdfInfo.ToPdfObject(),
		PdfVersion: pdfReader.PdfVersion().String(),
		Copyright:  "Copyright Example",
		Overwrite:  true,
	}
	
	// Store PDF Metadata into xmp document.
	err = xmpDoc.SetPdfInfo(xmpPdfMetadata)
	if err != nil {
		log.Fatalf("Err: %v", err)
	}
	
	// 1. Marshal XMP Document into raw bytes.
	metadataBytes, err := xmpDoc.MarshalIndent("", "\t")
	if err != nil {
		log.Fatalf("Err: %v", err)
	}
	
	// 2. Create new PdfStream
	metadataStream, err := core.MakeStream(metadataBytes, nil)
	if err != nil {
		log.Fatalf("Err: %v", err)
	}
	
	c.SetPdfWriterAccessFunc(func(w *model.PdfWriter) error {
		err := w.SetCatalogMetadata(metadataStream)
		if err != nil {
			return err
		}
		return nil
	})
	
	c.SetPdfWriterAccessFunc(func(w *model.PdfWriter) error {
		userPass := []byte(*opt.PDFPassword)
		ownerPass := []byte(*opt.PDFPassword)
		err := w.Encrypt(userPass, ownerPass, nil)
		if err != nil {
			return err
		}
		return nil
	})
	
	err = c.WriteToFile(outputPath)
	return nil
}

func setMetaData(pdf *gopdf.GoPdf) {
	pdf.SetInfo(gopdf.PdfInfo{
		Title:        "Metadata Baru",
		Author:       "Librantara Erlanga",
		Subject:      "Examle Metadata",
		Creator:      "Librantara",
		Producer:     "MajuTumbuhBersama",
		CreationDate: time.Now(),
	})
}

func initLicense() {
	// Make sure to load your metered License API key prior to using the library.
	//If you need a key, you can sign up and create a free one at https://cloud.unidoc.io
	check, err := unipdflicense.GetMeteredState()
	
	fmt.Println(check.Used)
	
	if check.OK {
		return
	}
	
	err = unipdflicense.SetMeteredKey(apiKey)
	if err != nil {
		fmt.Printf("ERROR: Failed to set metered key: %v\n", err)
	}
	
	// This example requires both for unioffice and unipdf.
	err = license.SetMeteredKey(apiKey)
	if err != nil {
		fmt.Printf("ERROR: Failed to set metered key: %v\n", err)
	}
}
