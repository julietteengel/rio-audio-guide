# -*- coding: utf-8 -*-
"""Merge all multilingual narration parts into one CSV + run duration-equivalence check."""
import csv
import sys

sys.path.insert(0, ".")

from narrations_data_part1 import DATA_PART1
from narrations_data_part2 import DATA_PART2
from narrations_data_part3 import DATA_PART3

# The 6-place validated sample (from narrations_multi_sample.csv), reused as-is.
SAMPLE = [
    {
        "name": "Cristo Redentor",
        "fr": "Au sommet du Corcovado, à plus de 700 mètres, le Christ Rédempteur ouvre les bras sur la baie depuis 1931. Haut de 38 mètres, imaginé par l'ingénieur Heitor da Silva Costa et sculpté par Paul Landowski, il est couvert de milliers d'écailles de pierre à savon. Élu l'une des sept nouvelles merveilles du monde, il veille sur Rio.",
        "en": "Atop Corcovado mountain, more than seven hundred meters up, Christ the Redeemer has opened his arms over the bay since nineteen thirty-one. Standing thirty-eight meters tall, designed by engineer Heitor da Silva Costa and sculpted by Paul Landowski, he's covered in thousands of soapstone tiles. Voted one of the New Seven Wonders of the World, he watches over Rio.",
        "es": "En la cima del Corcovado, a más de setecientos metros de altura, el Cristo Redentor abre los brazos sobre la bahía desde mil novecientos treinta y uno. Con treinta y ocho metros de altura, ideado por el ingeniero Heitor da Silva Costa y esculpido por Paul Landowski, está cubierto de miles de piezas de esteatita. Elegido una de las siete nuevas maravillas del mundo, vela por Río.",
        "pt": "No topo do Corcovado, a mais de setecentos metros, o Cristo Redentor abre os braços sobre a baía desde mil novecentos e trinta e um. Com trinta e oito metros de altura, idealizado pelo engenheiro Heitor da Silva Costa e esculpido por Paul Landowski, ele é coberto por milhares de pedras-sabão. Eleito uma das sete novas maravilhas do mundo, ele olha por todo o Rio.",
    },
    {
        "name": "Real Gabinete Português de Leitura",
        "fr": "Derrière une façade néo-manuéline inspirée du monastère des Hiéronymites de Lisbonne se cache l'une des plus belles bibliothèques du monde. Fondé en 1837 par la communauté portugaise, le Real Gabinete abrite quelque 350 000 ouvrages sous une nef de bois sculpté. Le magazine Time l'a classé quatrième plus belle bibliothèque de la planète. Levez les yeux.",
        "en": "Behind a neo-Manueline façade inspired by Lisbon's Jerónimos Monastery hides one of the most beautiful libraries in the world. Founded in eighteen thirty-seven by the Portuguese community, the Real Gabinete holds around three hundred fifty thousand books beneath a nave of carved wood. Time magazine ranked it the fourth most beautiful library on the planet. Look up.",
        "es": "Detrás de una fachada neomanuelina inspirada en el Monasterio de los Jerónimos de Lisboa se esconde una de las bibliotecas más bellas del mundo. Fundado en mil ochocientos treinta y siete por la comunidad portuguesa, el Real Gabinete alberga unos trescientos cincuenta mil libros bajo una nave de madera tallada. La revista Time lo clasificó como la cuarta biblioteca más bella del planeta. Levante la mirada.",
        "pt": "Atrás de uma fachada neomanuelina inspirada no Mosteiro dos Jerônimos de Lisboa se esconde uma das bibliotecas mais bonitas do mundo. Fundado em mil oitocentos e trinta e sete pela comunidade portuguesa, o Real Gabinete abriga cerca de trezentos e cinquenta mil livros sob uma nave de madeira entalhada. A revista Time o classificou como a quarta biblioteca mais bonita do planeta. Levante os olhos.",
    },
    {
        "name": "Cais do Valongo",
        "fr": "Ces vestiges de quai, redécouverts en 2011, comptent parmi les lieux les plus chargés d'histoire des Amériques : c'est ici que débarquèrent des centaines de milliers d'Africains réduits en esclavage, le plus grand point d'entrée de la traite du continent. Classé au patrimoine mondial de l'UNESCO en 2017, le site est aujourd'hui un mémorial. On s'y arrête en silence.",
        "en": "These wharf ruins, rediscovered in twenty eleven, rank among the most historically weighted sites in the Americas. This is where hundreds of thousands of enslaved Africans were forced ashore, the largest point of entry for the slave trade on the continent. Named a UNESCO World Heritage Site in twenty seventeen, it now stands as a memorial. People pause here in silence.",
        "es": "Estos vestigios del muelle, redescubiertos en dos mil once, se cuentan entre los lugares con más peso histórico de las Américas. Aquí desembarcaron cientos de miles de africanos esclavizados, el mayor punto de entrada de la trata en el continente. Declarado Patrimonio Mundial de la UNESCO en dos mil diecisiete, el sitio es hoy un memorial. Aquí uno se detiene en silencio.",
        "pt": "Esses vestígios de cais, redescobertos em dois mil e onze, estão entre os lugares mais carregados de história das Américas. Foi aqui que desembarcaram centenas de milhares de africanos escravizados, o maior ponto de entrada do tráfico no continente. Tombado como Patrimônio Mundial da UNESCO em dois mil e dezessete, o local é hoje um memorial. Aqui a gente para em silêncio.",
    },
    {
        "name": "Quinta da Boa Vista",
        "fr": "Ancien domaine de la famille impériale, ce vaste parc de São Cristóvão fut le jardin du palais où résidèrent rois et empereurs. Lacs, allées et bosquets en font un lieu de promenade populaire, qui abrite aujourd'hui le Museu Nacional et le BioParque. Le poumon vert du Rio impérial.",
        "en": "Once the imperial family's estate, this vast park in São Cristóvão was the palace garden where kings and emperors once lived. Lakes, pathways, and groves make it a popular place for a stroll, and it's now home to the Museu Nacional and the BioParque. The green lung of imperial Rio.",
        "es": "Antigua propiedad de la familia imperial, este extenso parque de São Cristóvão fue el jardín del palacio donde vivieron reyes y emperadores. Lagos, senderos y arboledas lo convierten en un lugar popular para pasear, y hoy alberga el Museu Nacional y el BioParque. El pulmón verde del Río imperial.",
        "pt": "Antigo domínio da família imperial, este grande parque de São Cristóvão foi o jardim do palácio onde viveram reis e imperadores. Lagos, alamedas e bosques fazem dele um lugar querido para passear, e hoje abriga o Museu Nacional e o BioParque. O pulmão verde do Rio imperial.",
    },
    {
        "name": "Bonde de Santa Teresa",
        "fr": "Le petit tramway jaune de Santa Teresa est le dernier de Rio, héritier d'une ligne du XIXe siècle. Il franchit les Arcos da Lapa puis grimpe en cliquetant les rues pavées du quartier bohème, entre ateliers d'artistes et maisons coloniales. Ce n'est pas qu'un transport : c'est une promenade, fenêtres ouvertes sur la ville.",
        "en": "The little yellow tram of Santa Teresa is the last one left in Rio, heir to a nineteenth-century line. It crosses the Arcos da Lapa, then clatters its way up the cobbled streets of this bohemian neighborhood, past artists' workshops and colonial houses. It's not just transport, it's a ride with the windows open to the city.",
        "es": "El pequeño tranvía amarillo de Santa Teresa es el último que queda en Río, heredero de una línea del siglo diecinueve. Cruza los Arcos da Lapa y luego sube traqueteando por las calles empedradas del barrio bohemio, entre talleres de artistas y casas coloniales. No es solo transporte, es un paseo con las ventanas abiertas a la ciudad.",
        "pt": "O bondinho amarelo de Santa Teresa é o último do Rio, herdeiro de uma linha do século dezenove. Ele cruza os Arcos da Lapa e sobe chacoalhando pelas ruas de paralelepípedo do bairro boêmio, entre ateliês de artistas e casas coloniais. Não é só transporte, é um passeio com as janelas abertas pra cidade.",
    },
    {
        "name": "Praia de Ipanema",
        "fr": "Immortalisée par la chanson « Garota de Ipanema », cette plage élégante est le rendez-vous branché de Rio. Chaque tronçon, repéré par un poste de secours, a sa tribu. Au coucher du soleil, tout le monde se tourne vers le Morro Dois Irmãos et applaudit le dernier rayon. Une tradition à vivre au moins une fois.",
        "en": "Made famous by the song 'The Girl from Ipanema', this elegant beach is Rio's trendiest gathering spot. Each stretch, marked by a lifeguard post, has its own crowd. At sunset, everyone turns toward the Morro Dois Irmãos and applauds the last ray of light. A tradition worth experiencing at least once.",
        "es": "Inmortalizada por la canción 'Garota de Ipanema', esta elegante playa es el punto de encuentro más de moda de Río. Cada tramo, marcado por un puesto de guardavidas, tiene su propia tribu. Al atardecer, todos se giran hacia el Morro Dois Irmãos y aplauden el último rayo de sol. Una tradición que vale la pena vivir al menos una vez.",
        "pt": "Eternizada pela música 'Garota de Ipanema', essa praia elegante é o point mais descolado do Rio. Cada trecho, marcado por um posto de salva-vidas, tem sua turma. No pôr do sol, todo mundo se vira pro Morro Dois Irmãos e aplaude o último raio de sol. Uma tradição pra viver pelo menos uma vez.",
    },
]

all_entries = SAMPLE + DATA_PART1 + DATA_PART2 + DATA_PART3

# dedupe by name, keep first occurrence (SAMPLE takes priority over part1 dup of Ipanema)
seen = set()
deduped = []
for e in all_entries:
    if e["name"] in seen:
        continue
    seen.add(e["name"])
    deduped.append(e)

print(f"Total unique entries: {len(deduped)}")

with open("narrations_multi_full.csv", "w", encoding="utf-8", newline="") as f:
    writer = csv.writer(f)
    writer.writerow(["name", "narration_fr", "narration_en", "narration_es", "narration_pt"])
    for e in deduped:
        writer.writerow([e["name"], e["fr"], e["en"], e["es"], e["pt"]])

print("Written to narrations_multi_full.csv")

# Duration-equivalence check (word count, +/-15% target, informational only)
print("\n--- Duration check (word counts) ---")
flagged = []
for e in deduped:
    fr_words = len(e["fr"].split())
    for lang in ("en", "es", "pt"):
        words = len(e[lang].split())
        ratio = words / fr_words
        if not (0.75 <= ratio <= 1.30):
            flagged.append((e["name"], lang, fr_words, words, ratio))

if flagged:
    print(f"{len(flagged)} entries outside +/-15-30% tolerance band:")
    for name, lang, fr_w, w, ratio in flagged:
        print(f"  {name} [{lang}]: fr={fr_w} words, {lang}={w} words, ratio={ratio:.2f}")
else:
    print("All entries within tolerance.")
