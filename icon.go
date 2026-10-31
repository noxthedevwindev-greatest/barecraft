package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// iconPNGB64 is the built-in 64x64 grass-block shortcut icon.
const iconPNGB64 = "iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAYAAACqaXHeAAAXCElEQVR42uXaZZce15UF4Bonk0xQiR3HY0fGxCPbiWWLGSxmbKlFrW4xWCyZZMsxU+yYmRnCzDjMs+YP3Xmf02tr9fyGfKh+q24d2Gefc7G6W/zY1e0v+er8ufMfhv4iryJg6l2XtT3fW9kG31/cjv6iv+36zvK2+vkpbeNrs9uq5ya3Da/Oajf9bqAd+NHatv+Ha6rd840/Wd92fLi0fukOvLuw9b0ys3T2fn9V2Vz+9IR28++3t6VPjmsrn51Udo/8fGNb88LUdusfh9rxX20uG4d+2teWPTW+bX9vUV2e4dj3g9Xlj+9F3x5b8qd+u61suYeH/f7X57Rb/jBY+Le+Pb/eb3pjbtnmiy36dF1swjjnwTGtu2jnOfWw4pmJBW7zmzfUrzZA1r00vYALkDMGOeYQQIbJbXlrXgW9+PHrCgD52/60o+whEQl+D/54XRHjAobO6T/vLALYj21Euo79clPb/d0VRQLfwPPLpwDXvzzjLBHsnfj1lsJ88jdby7fksIEcmHZ+tKz0/X55e9e6S/Z+ojIHAPYEIKscanfvV6BkGEaQgKK35InrS9c94gQqKxwLMCAFBCQSXAIjqxrcI2zogyUVgIS42BDotncW1K/3MsoODCpBcC7y2gTtPUxsaueHXRcsCLhgsEfANSfObWtfnFaAGSYoEAKUERHDnFBGRsqbvMwAJliBCBgIOt4pazp+ZVDALv5SqjIHtHY+BY5sNlUg4GTZZldidEV4PLtXhe7zHhYJkXE4tHuGXcznbe0RMOnMxRUwJgUDuBKWIc8MKTdZPvyzDaUsWAC9I8cB54IDMOC1saGd3ZQ0IpUnAsggEgYkaeOHXcRrFyS7wGdsUhHesa+0kSNIiSOfLsCWRPKZLguDbjf+9EWtu+Lgp6uBM4Y5QIDnDH4yoEI4QQYwHCgx98jTLTK4yJp7jCPMO1lmA5FA0E1fj64A+BQEv3zQyyAIl6BVC0wCZQ8B6bba6bDrHRK8V1meERfMun837e7LK1gvAGJEhjEHDIMpc0y6lA8Z44V7cgLiRDsZpGp3nzEho78A+NQlgFKqxhNlLjOqAB7gPdPx3sUn22RVFb9sIwZ+93RlXDLIZMbIGIdIhJsAuol3jK4HQIDK6MpI+qQyzXTFKAPAK1NAop+MyZZyEzzHmb4Q4koAMkMeeG0AIp4/z/zQQwS/frUjmYzgkKkyJU7W2U0XhZFvPiUqXSuzQ3WBUf1dNWSAAUjQ6fuY5ExWlKaL0fRJ2cugxpEsc6702STjWZkDIEjvZDaDKHswyDDSMvXKFmLJ8ENflhEGj0DZTSIkACECz9qGLlvi8E5y+GXn/IHeIDj3oasqaM6xBABBGeREP8YuQzKiDdPk3AuOLJmsBzhAFMAClUVEpD97l/4uMM+qKIQIKoubrDcEpD3diU6mZUSnHcEwqDxEw4oA8SEm4we90bs/3jpTASZdlCmGXcHLtHvKwKgMRmQHYOx7JxDspu96p50umyl/98hDFlAC8ws8grKYogsPglUInczvWd0JDsls0vE+MxM9bUiQ/QyM9CRabJfu+2TrxhwdVYwJNNMSxwkAMPcCkSUGBcv4yHlY4ALhLAOSeyVMn3wWI+4BzwALGJvp9wIIHmTzy54xQ/DIc/Evk3S8V23eS4DxDBl8eWYTkWJCFL2v7PrYcAUkqykxyp4ZpUyRQ1ngTH9ncOTUgggsCwLQrAqzPlcdSARSlvkgT5ZfwQKVaUyWZFv5syERmdOzXEYaPfLpamIhmzWIhKXLeJYMOvBPvvOS4Wkw5YYt4CgDkGWsADmWiUw7CEGS+8zTwMowp5wBlBUimfRxesALLKs9ABGastWvVVRGenqZBQSJuOwR+M1SOWsXOLXRFTCc9MXArkTOuv/K1ikDAWtISQNKEQkCQkIGLIbCNLlsTACW1SxZOQGSDY4RJNjsCbIoyiBLh88sjNhOpdCVMTIwZOAUZPqzwLSLI5sr5LHhnj67qpm8Svvbod4sMO+Ra87OjckcI8mYDKbPyZAyFFxGZ9WT6ZGNbJPT78hzmIWVgNhFKkB+gc/UqQ/DIDAYgNeWLsQPouARMFIQCZP7zArJfJb53o/ch/BtDdTNf/jKtu2t2W3Dt6bUNfhsbwT/7aZ26tcb6/6OP29vO96f347+rFdqry5t3/zHHW3LEzPb7X8aKJm1z0+o+/7HprUjP13TDv14Vdv6Zm8P8aOVpaNtz3cWt42vTK3fPS8vbvf929627oWJbf3Dk9rd/7yrHfv5urbmud4S95d9bdeLC9stv9vcbv39lnb03V5lvDa9bJ76aGPh0kaW7uGf9BZbf9hafnZ+sKBw8jf47g1t8+sz2iP/c6jw7n1lSflecc/Ykrv/3/e103/c1q7Y35sGp5y5sJ38VW8q+f6ytvuj3nT20qK2/Z257eAbK9qZvx+sdsJxBNBNv+mvgA68vrzAHPzhirbtqdnl5MAPlrcH/mN/kUJXIHf9U290fmB8AUPeuocmVtvGR6e2lfdeV2QI/PDbq8sWMje8PKX06fDDvt8j7/S2si8sqGRIkOTxR27vd5cUwavvH1fBw8sf4sTU98jkInL/a8vK55iBT7Xu2pOjyuhD/3WwHf/F+mJx6L157dH/PdJ2PD+/gGOTAcdIArz3X/cUwGQBy0gRLGJkCZl0tAGDwFX3Xd92fbiw/CFy3/eGK0rm/cJAXwUgZODtOfWrjSwbfMK2+fEZ9Qyj4O/5l92t76XJ1a7CJEObquYL0VufnFU4EcHfZXvPGa4AxjkBGDMAI6McvjipGMYaBwI7+eGGCkyFYBcJgJETBKJ0AfaAkVV6wKiMtQ9OKAKBkC2gVGH/q9Pag/95oKoDBuV66K1VVY1syfDA03Mq+3zpPsjSphro8KFryrSKQiJ8YoTBO5XHx7h95w2PAYLgCIDj76+v0lFK2JQx5QeMctdFZEy/1TWQgl1OgBc4YrQhSjtg2qucexXkErws0edXKQsQYYI48UFfVZ5A4AHcWIBg5AvGr8T4Vbn04d3+zNyyywacCFYNkqt6+TJWjN7ZmwVWPDW2gFSf6xnjRNkZODAMgEqowHsGZY/BlLVnTt1jnB3BGhwNhhVkr+T0e2AMQMpWIEoTSUCpGpUCg0vG2QX82HvrCpcksLHljZmlozKQzi8c9B7+7xvLh26CTKRIrvdDz80r37oiW6q/m3Ze1144saE9e3Rde2Bofrtv+w3t8QM9IPOubG/evr0dXnxNOzD/79qt63rrhNXj26u3bGnHl49t39qzpL1z54723LH17dv7l7eHdy1q9w7MbUeXfqO9eHJjO903pd2ydmK7fcPUartx4VX1fM+2Oe2JgyvbS6f6y8/9g/PaoUVXlwzbfD64Y0HZfGT34vJN/qGdC9uje5eWDVhh4INPWNm6c9OMek+On5Mrr29PHVpdlzhg/ObmmeXP/fprv9C6tV//fIHhQFAc3rVlVnv9tm1FyMEFYwqANsqIeP54X8lxSJfBOEcIh3SOLPl6kfHYvmWlg+gz/dPbsWXXll06gtT2ys2b25M3rirQiKD39OE1RdAzR9YWFja9f/mmTWWDX2SQlQyy7CLm1KpxdQ+f+zdOD9TzHRuntbfPDFVixN4t+9onyzkgnAiQILAAUWYQWKRwJpvuvQf+tvWTK4OAIUlWOLx76+wKXFXJLBLJC0KWBeaXDlD0ZYY997KGUPbc+1UZ8LIBM2yCQjYZ9xKgXZB800EU/RMrriu/2hdf3lsHrBzz6QqYI0G9dcdgBaukKACTksWy4DGfLAtSACoIaAHLHEfkVYpyfe3WrQXEO7YA4DeZVD3IRSp7LoSxoaTJ0meLDfreRdazpLFFnt2UPIyIYVs8fEnY+M/2BsGh6RefzQgFTpGgOxBSEYD5xSJHxgIEYRmb5OmrFjqcCxgY8uyrgOjLFLIEQi5ZQzwZevwnw3RlkyxiybhUDf/pGohNMm5eM6GwwKQdJoG7h1Villzx163rG/vFYoyiPgQY5xQEqZQBBgoIbHKuYnQZpBlwkORXebJHno0wHzL4EDg9AZPhI/2ebwEZNwSljYxqFBiyVapKpCNItlQGfAhypa/Digy+VIyLPFLnX9yrgH03fK0ClGFscQ6EUuHYM8PIkBGygAiKI0AZ9QuENtkkjySOlK6+DzBQKWlkCUY22NXGFtsCJweXZ1WXNkSzDxtdGUWi51QNHbglSeYlNlUnid6tuuozrZs0qqsywijwssQBRdniGPj0P8wCjVn3GTswzDDS9MWUIjINUGQBYVPWEMyXNnbJwAA8EjITCRQecp4RCifykmWkwOFZEvkngxgY+AxJAk8XmPyFXgVgHvAIciRoGRUUUAGW+Tkli2WMIst77JuT9VUBKduUuEwCiRgAM3jKuns+BIIwQXjnnl3dByEwZrBLlSICSXD4zRjEf3zo95KYtUswTP9Sj4BZF3QVFOMhAqhcjMk2A0oo/R5ZMkdXVuilz8sKMO5lgl3PZgjEyQKC2aSnzS8CkcwfMlLCgpQxwOkIVqAqgx1ygoQjFZnBkR6MmeIljazutPH63l5gw3Xn1iBBURYYJAy4QBnxTgCUgJUNwQMGjKpBFtAY5jwjMACyhgyA6fklo9LcC55tPgxy/AtCJSEt2BCT6Y2ujGYGSWJg02YAlST2stjyq6ph5k+1doPTRlcDZmQYMEwBw6ggBKjcOMQ2wgTqPste4ICi714mAWGDU7oqwyUDgmKTLrKQBweQgpBNlYNk+vzAlyVwVnx86cZk2Ea2WLJi5R8ucWTBxy9ZM2A3MOWisyO+fpNSVQkCd2Efy0BhGQEqw6939GQSy9lTaAvTbGM7JKgu8oB6Nka46LmyAtVPs9LLyM6frLIrcG3Ig0VwqRjVwzaiyLtUBb8Zw2Z+uTcGGGC8JKgR61ksUGCEUSXpHpCsqwWlCgDQzwRNBhHklCCAgCEuI7HqyWIpg68kCFqWVQQbiBJM7GQxBhffcJGDl79UEeLYIieh/EjYyKoyQNdeYOGl57TMBLIsCIoAykYGNZXAKRBK1HNGZOAByeAGNDCRz/JW9xhZUQIVXBYwMAgAKXwCjLBseMhlfBGQxCFT8OwLViW4gl+X4QNZkkQuY1B1AWAICkSmZAA4LOk72oEOs4yoBqxni4kIv4BmuwkcErIVVl26QGYMGfKezyygkC1bsUmXXzjYRq4q4jd2yMHMh0AFrl2Qnvnwju+sQskj0/jX2RBgXqYEndLKWkAwyl3JGGgwpw1whpMpbe6zhZaV7MkzDmSzo0L8eo+w7BQFkw2ZS5tLwIBLjPd8s8dGFmHIyg5RPEgkw5eKkACxSaKZ6exKcNvkC0uIoiAzCiODYlZvnMhU+moGL7qMAiDwVFCWq4IeOdBxnsUW0nQ5VUU3W1fBynjW/OQlJYsmchn0UqUjt+qpAkkgJxaEpmuqNL/9477Uup0zLy2HWVrmFAdYAQsmI7+gvENCyir7APJAZrTVR9lFKjmlSyebEtljG9m6Ex/8qzB+smbgjwyb6aZZ3+sucGWnmJMiScgaIDhj032qY+lXP9G6uRd1Z4+7DGSAKbnMqQwrU22cY1E5qQLGkQGgYGUv2ZQRGc7IDnwGSIEDwmfm6ZR+BlP3StY7WNhBsErL6RJdcshAXA4+6KiUvMuCKWsWeBFUR2JbJl5QWeEUEdnEyKisEVSmgsgWmHMkMMoJh/Sy2suqUFAyBLDM5Gwx6wGEhBR6wJETLCLoAssWf4KBib5EqSTEk2UXLpVMJuMM34kj06qKE/OM87vhQ9GMAZn7BYUAbVl8KJusFXLYAQCHMkoGAFkTjBIjmwVTzu8EmMxmDYA8+tnispf9RJbG2rKLow9bStuzdhizwsy2WDs/cKqSdMscudU0KGhZ5sxvDhgwLhuAaMtMgT1OMKnUGMrRU6aelGX6aMYZ5GUF594v4pCb9UhGfwEKHkbECJRNmSSLEDYElE2OauA/O9KsXrPYgwf52msvYCks25Q4ExhBGZA1wN1rp4RZsjJCLqvFHG1hPFMoElOWsu9XgLGdBVAIS6mzlXElPgWmjT1lrcIEjBBZz0GNd9nGZ0ueaZDPnG0YDzaNP791pgKgNCQAbCfDBkJM51gpIz8wjCMAUAFnVAcmuztlqNxyng+E9+wjiLx2vvjVJojs6lRWTnxlLJswuLKvZ4sMUsjBBBsZeF1ZHJHLCVSdClsOesgIDkhOfYDgNIsaz9mE5Lgrq7tkULnJKlLIuwcMyEyhCBUoMrzP6XOmWZWY7w35FiHwfHdAVA5nkKybZrDmi42QJbk5jiMLk4SIb9FlHxs+FicAhBLNOZ0+xUEMIsVz9vucZq+A8Rw9I0sGcyABKGIEJAAyQOQ0J+OIQE1xOeJiX1Up/1RZDk1lMF+lBO4+01+WxnAIOivXjD/8uZBtEditueZz/+/wE0iMK2dZygcFSoDndDenLEAKANBsYnJym0WNd/lKk02RqgHUO7ayuVFd+aIESz5t6X6qRWJkMlNtjuX8wpGvT3zBzZZq8CsuXTljxbpvjBruAoIAKvvq9K+M6vlMFTkGkAOUbGT/n66SQPK9L9mKDqKBZBtQmdN1cngSe3DQy5cc1ZPBM8tuCeIHDhnOYi7nEWQQl209UvlSVbUbtBjIgSHGgEICRQTko0OWt7JBzjtOGc33A2ULKKLIZAxJebqUfj7G8pG+jvwsuLLh4TMLrPTzfKzJxigfdGBLN+UXJhhhoYNYF5LY8b4ORPbMueLs5iPbRBkkQDk7sRx4Mjbyq29AypIMqw4AVELGEMGNnDWAzhl+vizldMh7ycixtqpiQzv/KipfsPONMvsLZNMhnyOwrCfyLcN72MSk+3fLr/ybCj6GCWShAbgsy5KsqYycx8lkdmTa6OgqWQipDM+uLF+ByPlfvg14zwawiMx9Fkd+dQPYsvtL384pcxZTSQxSsrtULfmwy664su2eem43vBvMPM14PnCmv0cZSVmq5iur5yxI6OYQJQcUAs0ncMHmm38OP9hEXo6zyMhkjsoMlFlBZooTaEbx7PboZeYiA7fnfA/MYJ4K0sbO3rlfHT4P0FcyNwOShZDfzP9Y01fzpVifz5og3xMQRw+AnAHkm6PAclKMXOWaPbyAs5rL1JbxJknI/oPvHHqkGkI8+/wnUMRmfMsX4yyTye6adVnrLAbCNFYwqnwwnmAZA0p3yFydL7X54CkgYPS99Ot8Fcp3OvI5XNUObPYFWfBkX5FT6HyDhCuDMluZHdI1sx+gL9uZ/rLlzsda5JM3cNY6wEKIYDYa+W8NgQLLsPYcMjKUbWoypuQ5zDdAAeQ7HL2sINO3U56Zw7MqBCqHqbpXgkeyKuBLQEjIFhvROX3K/w2wTy5nhyO7j7iyBa8DEethQQI+8ju67ITRHCpkuZz/C1Ji+fcUJGUudslgSMg8n09wOdyQ0ZHn/fm8lVWjMkVeZp7MHuyEMD4Qlv8yEUdIR4TqzRflDLKIMqvsmHFJ61Zf/dkKgkEB5CA0R9rAZ3cIpGCzfM25G1DKLp+4OEvlZLucac8lWzlf9A5J2c3lgCVfgcgLKnM6OzmQGbkKzAKJT77zcSTH8H5z0pV/5qpDUVtCjBJiQOZkJ7+YIpwFTUZZz5mrkaJEZcSFIOUOBBLJYZ9upkfZQQAZWcx/l6kAicjH2GyU2BVkPo5ksNaWEx8E5nAlZwXk4t9v/uNFt5DY+jhqMHBGbl6cc2HX/NuMrybjPtO1CZ/rmn+iELBnZ4jaVI5/MdGPOPSp2coKIPJkLDV3z7683m+f+pWy6wjO2oMfH2XY80wGBm3zRnfNAs3/LsDHD2wwwQr4xM8P+2OHD+t6Pv3TFz0kyjDc/hPEdA/Xgkv+qnz43wD4/g/e5QzfsSZFKgAAAABJRU5ErkJggg=="

func embeddedIcon() ([]byte, error) {
	return base64.StdEncoding.DecodeString(iconPNGB64)
}

func isPNG(b []byte) bool {
	return len(b) > 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}

// ensureIcon writes <root>/icons/<slug>.png (preferring a Modrinth project
// icon) plus a PNG-in-ICO variant for Windows shortcuts.
func ensureIcon(root, slug, modIconURL string) (pngPath, icoPath string, err error) {
	dir := filepath.Join(root, "icons")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	pngPath = filepath.Join(dir, slug+".png")
	icoPath = filepath.Join(dir, slug+".ico")

	var data []byte
	if modIconURL != "" {
		if b, e := getBytes(modIconURL); e == nil && isPNG(b) {
			data = b
		} else if e == nil {
			info("mod icon is not PNG; using built-in icon")
		}
	}
	if data == nil {
		if data, err = embeddedIcon(); err != nil {
			return "", "", fmt.Errorf("embedded icon: %w", err)
		}
	}
	if err = os.WriteFile(pngPath, data, 0o644); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(icoPath, pngICO(data), 0o644); err != nil {
		return "", "", err
	}
	return
}

// pngICO wraps PNG bytes in a single-image ICO container (valid since Vista).
func pngICO(png []byte) []byte {
	var w, h uint16 = 64, 64
	if len(png) >= 24 && string(png[12:16]) == "IHDR" {
		w16 := binary.BigEndian.Uint32(png[16:20])
		h16 := binary.BigEndian.Uint32(png[20:24])
		if w16 >= 256 {
			w = 0
		} else {
			w = uint16(w16)
		}
		if h16 >= 256 {
			h = 0
		} else {
			h = uint16(h16)
		}
	}
	out := make([]byte, 6+16)
	binary.LittleEndian.PutUint16(out[0:], 0) // reserved
	binary.LittleEndian.PutUint16(out[2:], 1) // type: icon
	binary.LittleEndian.PutUint16(out[4:], 1) // count
	out[6] = byte(w)
	out[7] = byte(h)
	out[8] = 0                                  // palette colors
	out[9] = 0                                  // reserved
	binary.LittleEndian.PutUint16(out[10:], 1)  // planes
	binary.LittleEndian.PutUint16(out[12:], 32) // bpp
	sz := uint32(len(png))
	binary.LittleEndian.PutUint32(out[14:], sz) // bytes in resource
	binary.LittleEndian.PutUint32(out[18:], 22) // offset to pixel data
	return append(out, png...)
}
