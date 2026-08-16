# -*- coding: utf-8 -*-
# Narrations multilingues (FR source + EN/ES/PT) -- partie 5, lieux groundes via
# le fallback Wikipedia lieu par lieu (enrich_grounding_geosearch.py) sur les
# lieux que le SPARQL en masse n'avait pas trouves. "id" correspond a la cle
# stable du pipeline de sourcing. "Catete Palace" (meme batiment que le deja
# narre "Palacio do Catete", a moins d'1m d'ecart) a ete volontairement omis
# pour eviter une narration redondante du meme lieu physique.

DATA_PART5 = [
{
"id": "wikidata:Lago do Campo de Santana:-22.90759:-43.18802",
"name": "Lago do Campo de Santana",
"fr": "Le Campo de Santana, aussi appelé Praça da República, tient son nom d'un fait précis : c'est tout près d'ici que fut proclamée la République du Brésil, en 1889. Un lac, blotti dans ce parc du centre de Rio, en garde le souvenir tranquille.",
"en": "Campo de Santana, also known as Praça da República, owes its name to a precise event: it was right nearby that the Republic of Brazil was proclaimed, in 1889. A lake, tucked into this park in downtown Rio, quietly keeps that memory.",
"es": "El Campo de Santana, también conocido como Praça da República, debe su nombre a un hecho preciso: fue muy cerca de aquí donde se proclamó la República de Brasil, en 1889. Un lago, escondido en este parque del centro de Río, guarda tranquilamente ese recuerdo.",
"pt": "O Campo de Santana, também conhecido como Praça da República, deve seu nome a um fato preciso: foi bem perto daqui que se proclamou a República do Brasil, em 1889. Um lago, escondido nesse parque do centro do Rio, guarda tranquilamente essa lembrança.",
},
{
"id": "overture:Polo Praça XV:-22.90282:-43.17458",
"name": "Polo Praça XV",
"fr": "Voici l'une des plus vieilles places de Rio, née au XVIe siècle. C'est ici, le 15 novembre 1889, que fut proclamée la République brésilienne — d'où son nom. Revitalisée en 2016 dans le cadre du Porto Maravilha, elle longe la baie de Guanabara et rassemble quatre monuments historiques, dont le chafariz du Mestre Valentim et la statue équestre de Dom João VI. Depuis 2011, c'est aussi, fait rare pour une place historique, un spot de skate autorisé.",
"en": "Here stands one of Rio's oldest squares, dating back to the sixteenth century. It was here, on November fifteenth, 1889, that the Brazilian Republic was proclaimed, which is where its name comes from. Revitalized in 2016 as part of Porto Maravilha, it runs along Guanabara Bay and gathers four historic monuments, including the Mestre Valentim fountain and the equestrian statue of Dom João VI. Since 2011, it's also, unusually for a historic square, an officially allowed skate spot.",
"es": "Aquí se encuentra una de las plazas más antiguas de Río, que data del siglo dieciséis. Fue aquí, el quince de noviembre de mil ochocientos ochenta y nueve, donde se proclamó la República brasileña, de ahí su nombre. Revitalizada en 2016 dentro del Porto Maravilha, bordea la bahía de Guanabara y reúne cuatro monumentos históricos, entre ellos la fuente del Mestre Valentim y la estatua ecuestre de Dom João VI. Desde 2011, además, algo poco común en una plaza histórica, es un lugar de patinaje autorizado.",
"pt": "Aqui está uma das praças mais antigas do Rio, que existe desde o século dezesseis. Foi aqui, em 15 de novembro de 1889, que se proclamou a República brasileira, daí seu nome. Revitalizada em 2016 no âmbito do Porto Maravilha, ela margeia a Baía de Guanabara e reúne quatro monumentos históricos, entre eles o chafariz do Mestre Valentim e a estátua equestre de Dom João VI. Desde 2011, aliás, um caso raro para uma praça histórica, é também um point de skate autorizado.",
},
{
"id": "overture:Vasco da Gama, Rio de Janeiro:-22.89194:-43.22695",
"name": "Vasco da Gama, Rio de Janeiro",
"fr": "Ce quartier porte le nom du navigateur portugais Vasco de Gama — en hommage au centenaire du club de football CR Vasco da Gama, en 1998. C'est ici que se trouve le stade São Januário, temple du club. Jusqu'en 1997, le quartier faisait partie de São Cristóvão.",
"en": "This neighborhood bears the name of Portuguese explorer Vasco da Gama, in honor of the centenary of the CR Vasco da Gama football club, in 1998. This is where the club's stadium, São Januário, stands. Until 1997, the area was part of São Cristóvão.",
"es": "Este barrio lleva el nombre del navegante portugués Vasco da Gama, en homenaje al centenario del club de fútbol CR Vasco da Gama, en 1998. Aquí se encuentra el estadio del club, São Januário. Hasta 1997, el barrio formaba parte de São Cristóvão.",
"pt": "Esse bairro leva o nome do navegador português Vasco da Gama, em homenagem ao centenário do clube de futebol CR Vasco da Gama, em 1998. É aqui que fica o estádio São Januário, casa do clube. Até 1997, o bairro fazia parte de São Cristóvão.",
},
{
"id": "overture:Palácio de são cristóvão:-22.90583:-43.22445",
"name": "Palácio de são cristóvão",
"fr": "Résidence de la famille royale portugaise puis des empereurs du Brésil jusqu'en 1889, ce palais abritait 92,5% des collections du Musée national — jusqu'à l'incendie du 2 septembre 2018, qui a détruit en grande partie le bâtiment. Rouvert après restauration en 2025.",
"en": "Home to the Portuguese royal family and then to Brazil's emperors until eighteen eighty-nine, this palace held ninety-two point five percent of the collections of the National Museum, until the fire of September second, twenty eighteen, which largely destroyed the building. Reopened after restoration in twenty twenty-five.",
"es": "Residencia de la familia real portuguesa y luego de los emperadores de Brasil hasta 1889, este palacio albergaba el noventa y dos coma cinco por ciento de las colecciones del Museo Nacional, hasta el incendio del dos de septiembre de 2018, que destruyó gran parte del edificio. Reabierto tras su restauración en 2025.",
"pt": "Residência da família real portuguesa e depois dos imperadores do Brasil até 1889, esse palácio abrigava noventa e dois vírgula cinco por cento das coleções do Museu Nacional, até o incêndio de 2 de setembro de 2018, que destruiu grande parte do edifício. Reaberto após restauração em 2025.",
},
{
"id": "overture:Fazenda Imperial de Santa Cruz:-22.91000:-43.68531",
"name": "Fazenda Imperial de Santa Cruz",
"fr": "D'abord domaine et couvent jésuite dès 1570, cette propriété devint résidence des vice-rois portugais, puis palais royal à l'arrivée de la cour en 1808. Le prince régent Pedro Ier y passa sa lune de miel avec la princesse Leopoldina en 1818. Après la proclamation de la République, en 1889, elle fut transformée en école militaire — une vocation qu'elle garde aujourd'hui.",
"en": "First a Jesuit estate and convent from fifteen seventy, this property became a residence for the Portuguese viceroys, then a royal palace when the court arrived in eighteen-oh-eight. Prince Regent Pedro the First spent his honeymoon here with Princess Leopoldina in eighteen eighteen. After the proclamation of the Republic in eighteen eighty-nine, it was turned into a military academy, a role it still holds today.",
"es": "Primero hacienda y convento jesuita desde 1570, esta propiedad se convirtió en residencia de los virreyes portugueses, y luego en palacio real cuando llegó la corte en 1808. El príncipe regente Pedro I pasó aquí su luna de miel con la princesa Leopoldina en 1818. Tras la proclamación de la República, en 1889, se transformó en academia militar, función que conserva hoy.",
"pt": "Primeiro fazenda e convento jesuíta desde 1570, essa propriedade se tornou residência dos vice-reis portugueses, e depois palácio real com a chegada da corte em 1808. O príncipe regente Dom Pedro I passou aqui sua lua de mel com a princesa Leopoldina em 1818. Após a proclamação da República, em 1889, foi transformada em escola militar, função que mantém até hoje.",
},
{
"id": "overture:Museu de Sitio Arqueologico da Antiga Se:-22.90390:-43.17582",
"name": "Museu de Sitio Arqueologico da Antiga Se",
"fr": "On l'appelle simplement Antiga Sé : cette paroisse fut le siège épiscopal de Rio jusqu'en 1976, date d'achèvement de la nouvelle cathédrale. Face à la Praça XV, elle voisine les bâtiments coloniaux de l'ancien Convento do Carmo et de l'église de l'Ordre Tiers du Carmo.",
"en": "Simply known as Antiga Sé, this parish was Rio's episcopal seat until 1976, when the new cathedral was completed. Facing Praça XV, it stands beside the colonial buildings of the former Convento do Carmo and the church of the Third Order of Carmo.",
"es": "Se le conoce simplemente como Antiga Sé: esta parroquia fue la sede episcopal de Río hasta 1976, fecha en que se terminó la nueva catedral. Frente a la Praça XV, es vecina de los edificios coloniales del antiguo Convento do Carmo y de la iglesia de la Orden Tercera del Carmo.",
"pt": "É conhecida simplesmente como Antiga Sé: essa paróquia foi a sede episcopal do Rio até 1976, quando foi concluída a nova catedral. De frente para a Praça XV, é vizinha dos prédios coloniais do antigo Convento do Carmo e da Igreja da Ordem Terceira do Carmo.",
},
{
"id": "overture:Christ Redeemer, Rio de Janeiro, Brazil:-22.95165:-43.21084",
"name": "Christ Redeemer, Rio de Janeiro, Brazil",
"fr": "Trente mètres de béton armé et de pierre ollaire, vingt-huit mètres d'envergure : cette statue Art déco du Christ, sculptée par le Français Paul Landowski et bâtie par le Brésilien Heitor da Silva Costa entre 1922 et 1931, domine Rio du haut du Corcovado, à 700 mètres. La plus grande sculpture Art déco du monde, élue parmi les sept nouvelles merveilles du monde.",
"en": "Thirty meters of reinforced concrete and soapstone, twenty-eight meters wide: this Art Deco statue of Christ, sculpted by Frenchman Paul Landowski and built by Brazilian Heitor da Silva Costa between nineteen twenty-two and nineteen thirty-one, looms over Rio from the top of Corcovado, seven hundred meters up. The largest Art Deco sculpture in the world, voted one of the New Seven Wonders.",
"es": "Treinta metros de hormigón armado y esteatita, veintiocho metros de envergadura: esta estatua Art Déco de Cristo, esculpida por el francés Paul Landowski y construida por el brasileño Heitor da Silva Costa entre 1922 y 1931, domina Río desde lo alto del Corcovado, a 700 metros. La escultura Art Déco más grande del mundo, elegida una de las nuevas siete maravillas.",
"pt": "Trinta metros de concreto armado e pedra-sabão, vinte e oito metros de envergadura: essa estátua Art Déco de Cristo, esculpida pelo francês Paul Landowski e construída pelo brasileiro Heitor da Silva Costa entre 1922 e 1931, domina o Rio do alto do Corcovado, a 700 metros. A maior escultura Art Déco do mundo, eleita uma das novas sete maravilhas.",
},
{
"id": "overture:National Museum of Brazil:-22.90577:-43.22653",
"name": "National Museum of Brazil",
"fr": "Fondé en 1818 par le roi Jean VI, c'est la plus ancienne institution scientifique du Brésil : plus de vingt millions d'objets, l'une des plus grandes collections d'histoire naturelle et d'anthropologie au monde. Un incendie l'a ravagé le 2 septembre 2018 ; en 2019, plus de trente mille objets de la famille impériale ont été retrouvés lors de fouilles archéologiques dans le zoo voisin.",
"en": "Founded in eighteen eighteen by King João the Sixth, this is Brazil's oldest scientific institution: more than twenty million objects, one of the largest natural history and anthropology collections in the world. A fire ravaged it on September second, twenty eighteen; in twenty nineteen, more than thirty thousand objects from the imperial family were recovered during archaeological digs at the neighboring zoo.",
"es": "Fundado en 1818 por el rey Juan Sexto, es la institución científica más antigua de Brasil: más de veinte millones de objetos, una de las mayores colecciones de historia natural y antropología del mundo. Un incendio lo devastó el dos de septiembre de 2018; en 2019, más de treinta mil objetos de la familia imperial fueron encontrados durante excavaciones arqueológicas en el zoológico vecino.",
"pt": "Fundado em 1818 pelo rei Dom João Sexto, é a mais antiga instituição científica do Brasil: mais de vinte milhões de objetos, uma das maiores coleções de história natural e antropologia do mundo. Um incêndio o devastou em 2 de setembro de 2018; em 2019, mais de trinta mil objetos da família imperial foram encontrados em escavações arqueológicas no zoológico vizinho.",
},
{
"id": "overture:Fort Copacabana:-22.98618:-43.18653",
"name": "Fort Copacabana",
"fr": "À la pointe de Copacabana, ce fort accueille environ dix mille visiteurs par mois — l'un des plus beaux points de vue de la ville. Officiellement Musée historique de l'armée, on choisit entre la visite restreinte, aux espaces extérieurs, ou complète, qui donne accès à l'intérieur du fort et à son musée.",
"en": "At the tip of Copacabana, this fort welcomes around ten thousand visitors a month, one of the city's finest viewpoints. Officially the Army Historical Museum, visitors can choose between the restricted visit, limited to the outdoor areas, or the full visit, which gives access to the inside of the fort and its museum.",
"es": "En la punta de Copacabana, este fuerte recibe cerca de diez mil visitantes al mes, uno de los mejores miradores de la ciudad. Oficialmente Museo Histórico del Ejército, se puede elegir entre la visita restringida, a las áreas exteriores, o la completa, que da acceso al interior del fuerte y a su museo.",
"pt": "Na ponta de Copacabana, esse forte recebe cerca de dez mil visitantes por mês, um dos melhores mirantes da cidade. Oficialmente Museu Histórico do Exército, dá para escolher entre a visita restrita, às áreas externas, ou a completa, que dá acesso ao interior do forte e ao seu museu.",
},
{
"id": "overture:Museu do paço:-22.90289:-43.17331",
"name": "Museu do paço",
"fr": "Construit au XVIIIe siècle pour loger les gouverneurs, ce bâtiment devint le siège administratif du vice-roi, puis du roi Jean VI, puis des empereurs du Brésil. Considéré comme le plus important édifice civil colonial du pays, c'est aujourd'hui un centre culturel.",
"en": "Built in the eighteenth century to house the governors, this building went on to serve as the administrative seat of the viceroy, then of King João the Sixth, then of Brazil's emperors. Regarded as the country's most important colonial civil building, it's now a cultural center.",
"es": "Construido en el siglo dieciocho para alojar a los gobernadores, este edificio pasó a ser sede administrativa del virrey, luego del rey Juan Sexto, y después de los emperadores de Brasil. Considerado el edificio civil colonial más importante del país, hoy es un centro cultural.",
"pt": "Construído no século dezoito para abrigar os governadores, esse prédio passou a ser sede administrativa do vice-rei, depois do rei Dom João Sexto, e depois dos imperadores do Brasil. Considerado o mais importante edifício civil colonial do país, hoje é um centro cultural.",
},
{
"id": "overture:Sugarloaf Mountain:-22.94975:-43.15623",
"name": "Sugarloaf Mountain",
"fr": "Ce pain de sucre de granit s'élève à 396 mètres au-dessus du port, à l'entrée de la baie de Guanabara. Son téléphérique et sa vue panoramique en ont fait un symbole mondial de Rio. Protégé depuis 2006 en tant que monument naturel, il est inscrit au patrimoine mondial de l'UNESCO depuis 2012.",
"en": "This granite sugarloaf rises three hundred ninety-six meters above the harbor, at the mouth of Guanabara Bay. Its cable car and panoramic views have made it a world symbol of Rio. Protected since two thousand six as a natural monument, it's been a UNESCO World Heritage Site since two thousand twelve.",
"es": "Este pan de azúcar de granito se eleva trescientos noventa y seis metros sobre el puerto, en la entrada de la bahía de Guanabara. Su teleférico y sus vistas panorámicas lo han convertido en un símbolo mundial de Río. Protegido desde 2006 como monumento natural, es Patrimonio Mundial de la UNESCO desde 2012.",
"pt": "Esse pão de açúcar de granito se eleva 396 metros acima do porto, na entrada da Baía de Guanabara. Seu bondinho e suas vistas panorâmicas o transformaram em símbolo mundial do Rio. Protegido desde 2006 como monumento natural, é Patrimônio Mundial da UNESCO desde 2012.",
},
]
