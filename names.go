package main

import (
	"math/rand/v2"
	"strings"
)

var firstNames = strings.Fields(`
	Abner Agatha Ambrose Barnaby Bartholomew Beatrix Bertram Blanche Cornelius Crispin
	Cyrus Delphine Desmond Dorcas Ebenezer Edmund Elspeth Enoch Ephraim Erasmus
	Esmeralda Eudora Eustace Ezekiel Fenwick Florian Gideon Gertrude Godfrey Griselda
	Gwendolyn Hagar Hamish Hepzibah Hezekiah Horatio Ichabod Ignatius Isadora Jasper
	Jedediah Jeremiah Josiah Lazarus Lenore Leopold Lucius Lucretia Mabel Malachi
	Marigold Mortimer Mordecai Morwenna Nathaniel Nehemiah Obadiah Octavia Odessa Ophelia
	Orville Osgood Oswald Percival Philomena Prudence Quentin Quincy Rosalind Rufus
	Rupert Salome Septimus Silas Solomon Tabitha Thaddeus Theodora Thorne Ulysses
	Ursula Vesper Virgil Wendell Wilhelmina Winifred Wolcott Zachariah Zebedee Zillah
	Augustin Benedict Clementine Drusilla Elijah Fergus Hester Ingram Jethro Lavinia
`)

var lastNames = strings.Fields(`
	Ashgrove Barrowman Blackthorn Bonehill Briarwood Cadaver Coffinmaker Coldwell Cragmoor Crowley
	Cryptkeeper Darkmoor Deadwood Digby Dirgeworth Dreadmore Duskwater Embalm Fogwell Gallowglass
	Ghastly Gloomsbury Gorehound Gravelock Gravesend Graveworth Grimsby Grimwald Hallow Hearsey
	Hollowell Ironcask Lanternjaw Lichfield Marrowbone Marsh Moldwell Morgue Mortmain Mossgrave
	Mournwood Nightshade Oakenshroud Ossuary Palebrook Pallbearer Peatmoss Pickaxe Plaguewood Quietus
	Ravenscroft Rattlebone Restwell Rotwood Rookwood Rustgate Sallow Sepulcher Shadwell Shovelton
	Shroud Skelton Slabstone Smolder Spadework Stillwater Stonecold Tallowmere Thornbury Tombstone
	Trench Umbridge Undertow Vault Vexley Wakefield Wailing Weepwillow Whitbone Widdershins
	Wickham Wormwood Wraithmoor Yewdale Ashcombe Bleakley Candlewick Deathridge Dunmore Ebonhurst
	Fennick Grubb Hemlock Knell Larkspur Mausley Nethercott Plotkin Sexton Wolfsbane
`)

// randomPlayerName combines a random first and last name.
func randomPlayerName() string {
	return firstNames[rand.IntN(len(firstNames))] + " " + lastNames[rand.IntN(len(lastNames))]
}
