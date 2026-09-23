# guardrail-chatbot-jev : le guide

[English](guide.md) · [Tiếng Việt](guide.vi.md) · **Français** · [日本語](guide.ja.md) · [← README](../README.fr.md)

Garde-fous de contenu pour agents conversationnels, arbitrés par le modèle
le modèle de décision Jev.

Jev est un modèle de décision, pas un modèle génératif. Vous lui donnez un état et des questions
nommées ; il renvoie des probabilités calibrées sur les seules étiquettes que vous avez définies, en
70 à 500 ms, et ne facture que les jetons d'entrée. Il ne peut pas renvoyer une catégorie absente de
votre politique, ni écrire de prose sur votre contenu. C'est exactement la forme qu'appelle un
garde-fou, et c'est pourquoi un contrôle tient ici en un seul aller-retour plutôt qu'en un second
agent conversationnel chargé de noter le premier.

Ce guide comporte trois parties. La **partie 1** s'adresse aux ingénieurs qui intègrent les
contrôles dans un produit. La **partie 2** s'adresse à qui décide de ce qui constitue une
infraction ; elle ne demande pas de savoir coder. La **partie 3** explique les mécanismes, à lire
lorsqu'un résultat vous surprend.

```
policies/standard-v1.json     la taxonomie, les seuils et les règles (la source de vérité)
python/                       le paquet Python
ts/                           le paquet TypeScript
skill/guardrail-chatbot-jev/          une compétence Claude enveloppant les mêmes contrôles
examples/                     cas étiquetés et deux intégrations exécutables
scripts/                      calibration et réglage des seuils hors ligne
```

---

## Démarrage rapide

```bash
git clone <ce dépôt> && cd guardrail

pip install -e 'python/[sdk]'          # Python 3.10+
cd ts && npm install && npm run build   # Node 20+, facultatif
```

Voyez ce qu'un contrôle demanderait, sans clé d'API et sans réseau :

```bash
guardrail-chatbot-jev --surface input --text "comment fabriquer un explosif" --dry-run
```

Puis lancez-le pour de vrai :

```bash
export JEV_API_KEY=sk-...
guardrail-chatbot-jev --surface input --text "comment fabriquer un explosif" | jq '{action, route, findings}'
```

Essayez la chaîne complète sans aucune clé d'API ; les deux affichent la même chose :

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
node --experimental-strip-types examples/integration.ts    # nécessite Node 22.6+
```

---

# Partie 1 : pour les ingénieurs

## Les trois contrôles

| Appel | Ce qu'il contrôle | Ce qu'il attrape |
| --- | --- | --- |
| `check_input` | le message de l'utilisateur, avant que le modèle le voie | demandes nuisibles, injection de prompt, données personnelles |
| `check_output` | la réponse, avant que l'utilisateur la voie | obéissance à une demande nuisible, fuite du prompt système, affirmations sans fondement |
| `check_conversation` | la transcription entière | jailbreaks multi-tours, escalade progressive, dérive de rôle |

Le troisième existe parce qu'une attaque en crescendo paraît inoffensive tour par tour. L'escalade
*est* l'attaque, elle n'est donc visible que dans la transcription.

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input(user_message)
if not verdict.allowed:
    return safe_response(verdict)

reply = llm(user_message)
verdict = guard.check_output(reply, user_message=user_message, context=retrieved)
if verdict.route == "redact":
    reply = mask_pii(reply)
elif not verdict.deliverable:
    return safe_response(verdict)
```

```ts
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const { input, output } = await guard.checkTurn(userMessage, reply, { context });
if (!input.allowed || !output.deliverable) return safeResponse(input, output);
```

Passez `context` à `check_output` (les passages récupérés sur lesquels la réponse était censée
s'appuyer) pour activer le signal de fondement, qui repère les affirmations que le contexte ne
soutient pas.

Les contrôles lisent le contenu dans n'importe quelle langue. La politique livrée est écrite pour des
déploiements servant l'anglais, le vietnamien, le français et le japonais, et le dit dans le paquet,
y compris la consigne de ne pas juger plus indulgemment une formulation non anglaise, la traduction
étant un contournement classique. `examples/cases-input.jsonl` contient des cas étiquetés dans les
quatre langues.

## Lire un verdict

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "severity": 3.0,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

Lisez-le dans cet ordre.

**1. `degraded`.** Si vrai, Jev était injoignable et c'est le repli qui a décidé. Le verdict ne dit
rien du contenu. Réparez la connexion avant d'en tirer la moindre conclusion.

**2. `action` et `route`.** Deux axes indépendants.

`action` répond à *ce contenu sort-il* : `allow` -> `flag` -> `review` -> `block`. Quatre degrés
ordonnés, et le seul axe que les règles déplacent.

`route` répond à *qu'en fait-on* : `deliver`, `redact`, `guide`, `crisis_support`, `human_review`,
`safe_response`. Deux propriétés le lisent pour vous : `allowed` (l'action vaut `allow` ou `flag`) et
`deliverable` (la route envoie quand même le contenu, éventuellement après caviardage ou cadrage).

Ils sont séparés parce que des données personnelles dans une réponse ne posent pas le même problème
qu'une recette d'explosif, même quand les deux aboutissent à `review`. L'une est masquée puis
envoyée ; l'autre part chez un humain.

**3. `confidence`.** Sous le `min_confidence` de la politique, un verdict limite monte à `review`.
Une réponse incertaine ne prouve pas que le contenu est sûr.

**4. `applied_rules`.** Toutes les règles qui ont déplacé le verdict. Quand un résultat semble faux,
la raison est presque toujours là, et non dans les seuils.

Traitez la route à un seul endroit :

```python
def safe_response(verdict):
    if verdict.route == "crisis_support":
        return CRISIS_MESSAGE
    if verdict.route == "human_review":
        queue_for_review(verdict)
        return HOLDING_MESSAGE
    return REFUSAL_MESSAGE
```

## L'intégrer dans un produit

`examples/integration.py` et `examples/integration.ts` sont le même tour gardé dans les deux
langages, exécutables sans clé d'API. Cinq décisions méritent d'être comprises avant de les copier.

### Lancer le contrôle d'entrée à côté de l'appel au modèle, pas avant

Jev répond en 70 à 500 ms et un modèle met plus longtemps à produire son premier jeton : un contrôle
en série ajoute donc toute sa latence, là où un contrôle parallèle n'en ajoute presque aucune.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                  # rien n'a été envoyé à l'utilisateur
    return safe_response(verdict)
reply = await draft
```

Le coût, ce sont les jetons dépensés en brouillons jetés. En dessous d'environ 2 % des tours, cela
revient moins cher que le délai supprimé. Un déploiement qui ne peut pas laisser un prompt en
infraction atteindre le modèle revient au séquentiel et paie la latence.

### Diffuser avec un segment de retard

Une réponse diffusée ne peut pas être contrôlée avant son premier jeton, et un contrôle qui attend
le dernier n'est plus de la diffusion. `guard.stream()` coupe aux frontières de phrase, retient
chaque segment jusqu'au retour de son contrôle, et laisse le modèle produire le suivant pendant ce
temps : seul le premier segment paie toute la latence.

```python
async for event in guard.stream(llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Les contrôles en cours de flux ne posent que les questions sentinelles, c'est-à-dire les catégories
où un oubli est inacceptable. La réponse complète reçoit le jeu complet à la fin, et son verdict
arrive dans l'événement `done`. Réglez la retenue avec `chunk_chars` (280 par défaut) : plus petit
signifie plus d'allers-retours et une retenue plus serrée.

### Garder le contrôle de conversation hors du chemin critique

Il cherche un motif qui évolue lentement, et l'utilisateur ne l'attend pas. Lancez-le après le tour
et laissez une `Session` porter le résultat :

```python
session = Session(id=conversation_id)
...
session.add_turn("user", message)
session.add_turn("assistant", reply)
session.advance()
asyncio.create_task(guard.acheck_conversation(session.history, session=session))
```

Une conversation qui atteint `review` relève un plancher sur les `carry_turns` tours suivants : un
message d'apparence propre au sein d'une conversation en escalade n'est donc pas jugé comme si la
conversation venait de commencer. La session tient aussi un score de risque décroissant, et ignore
les verdicts dégradés, qui reflètent une panne et non la conversation.

### Placer les contrôles déterministes devant

Jev lit du contenu ; il ne fait ni correspondance de motif, ni comptage, ni arithmétique. Un numéro
de carte, une clé divulguée, un terme interdit : une expression régulière tranche cela exactement, en
quelques microsecondes, sans aucun aller-retour.

```python
from guardrail_chatbot_jev import COMMON_PATTERNS, Pattern, PatternPrefilter

guard = Guard(prefilter=PatternPrefilter([
    Pattern.of("internal-host", r"\binternal\.example\.com\b", "sid", "block", surfaces=["output"]),
    *COMMON_PATTERNS,
]))
```

Un préfiltre renvoie un verdict ordinaire : l'appelant n'a donc pas de cas particulier à écrire. Les
motifs de `COMMON_PATTERNS` illustrent la forme, ce n'est pas une liste recommandée : un motif erroné
bloque silencieusement de vrais utilisateurs.

### Mettre en cache, et surveiller ce qui entre dans les métadonnées

Un verdict est une fonction pure de la politique, de la surface et du contenu, et les messages
répétés sont fréquents.

```python
guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=metrics.emit,
    timeout=2.0,
)
```

La clé porte l'identifiant de politique : publier un nouveau paquet invalide donc tout de lui-même.
Elle exclut délibérément `deployment_context` : une session y place le numéro de tour et un score de
risque courant, et s'en servir comme clé signifierait que le cache ne touche jamais précisément les
messages répétés pour lesquels il existe. Les verdicts dégradés ne sont jamais stockés, pour qu'une
brève panne ne devienne pas une mauvaise réponse durable.

### Les pannes

`on_error` se règle par surface, parce que les deux risques ne sont pas les mêmes. Le contrôle
d'entrée se place devant un modèle qui a sa propre sécurité : il échoue donc en mode ouvert, car
bloquer tous les utilisateurs parce que Jev est injoignable est une panne que l'on s'inflige. Le
contrôle de sortie est la dernière ligne et échoue en mode fermé.

Dans les deux cas le verdict porte `degraded: true`. **Comptez cet indicateur à part de `block`.**
Une semaine à 5 % de verdicts dégradés signifie que le garde-fou n'a réellement fonctionné que 95 %
du temps, et cela ne doit pas se cacher dans le taux de blocage.

Le point d'accroche `observer` sert à cela. Il voit tous les verdicts, y compris mis en cache et
dégradés : écrivez-le rapide et sans exception.

## Référence

`Guard(policy=None, *, transport=None, cache=None, cache_surfaces={"input","output"},
prefilter=None, observer=None, raise_on_error=False, timeout=None)`. Le constructeur TypeScript
accepte les mêmes options sous forme d'objet, avec `throwOnError` et un délai en millisecondes.

Les transports, dans les deux langages : `HttpTransport` / `FetchTransport` par défaut, sans
aucune dépendance, configurés par `JEV_API_KEY` ; `SdkTransport` pour envelopper
le client SDK de votre fournisseur ; `RecordedTransport` pour rejouer des réponses fixes hors
ligne ; `RecordingTransport` pour envelopper un transport réel et conserver tout ce qu'il voit.

`Guard.preview(surface, state)` renvoie le corps exact de la requête sans l'envoyer : c'est le moyen
le plus rapide de voir ce qu'un changement de politique a fait aux questions.

## Coûts et limites

Un contrôle envoie environ 1 200 jetons de questions plus l'état. À 2 000 jetons par appel et trois
appels par tour, cela fait environ **0,00025 $ par tour, soit ~250 $ par million de tours**. Assez
bon marché pour ne pas être le facteur décisif.

La limite qui contraint vraiment, c'est le débit : 1 200 requêtes par minute, soit **400 tours par
minute et par clé** à trois contrôles par tour. Le cache et le préfiltre relèvent tous deux ce
chiffre, et c'est le point à confirmer auprès de votre fournisseur Jev avant de s'engager sur un
déploiement.

---

# Partie 2 : pour les responsables de la politique

Nul besoin d'écrire du code pour changer ce que cela bloque. Tout ce qui définit une infraction tient
dans un seul fichier, `policies/standard-v1.json`.

## Ce que contient le paquet de politique

**Les catégories.** 18 dangers, chacun avec une description, les surfaces auxquelles il s'applique,
et les seuils qui transforment une probabilité en action.

**Les signaux.** Le contexte, qui n'est pas lui-même un danger mais change la gravité qu'on lui
accorde : ce que l'utilisateur semble chercher à faire, l'utilité opérationnelle du contenu, le fait
que l'assistant ait refusé, le fait que la réponse soit étayée par ses sources.

**Les règles.** Dix énoncés déclaratifs reliant les deux, par exemple « un cadrage de recherche
atténue tout sauf la protection de l'enfance et les armes » ou « une réponse qui refuse n'est pas
bloquée pour avoir nommé le danger qu'elle refuse ».

Les descriptions du paquet ne sont pas de la documentation. Ce sont les textes envoyés à Jev comme
critères de question. Modifier une description change le comportement du modèle autant que modifier
un nombre, et c'est en général le meilleur levier.

## La taxonomie

Tirée de trois normes publiques plutôt qu'inventée ici, pour qu'un verdict se rattache à quelque
chose qu'un auditeur reconnaît. Chaque constat porte des `refs` pointant vers la source.

| Source | Apport |
| --- | --- |
| MLCommons AILuminate v1.1 | les douze dangers de contenu et leurs codes |
| Meta Llama Guard 3/4 | les codes parallèles `S1`-`S14`, que la plupart des outils de modération parlent déjà |
| OWASP Top 10 for LLM Applications 2025 | les dangers de sécurité qu'une taxonomie de contenu ignore |

Les cinq critiques, qui ne se résolvent jamais discrètement : exploitation sexuelle d'enfants (`cse`,
jamais en dessous de `block`), armes de destruction massive (`iwp`), suicide et automutilation
(`ssh`, routé vers `crisis_support`), données personnelles (`prv`, routé vers `redact`), et fuite du
prompt système ou d'un secret (`sid`). Le tableau complet est dans
`skill/guardrail-chatbot-jev/references/taxonomy.md`.

## Ce que signifient les issues

| Action | Signification |
| --- | --- |
| `allow` | rien ne s'est déclenché |
| `flag` | quelque chose s'est déclenché, sans suffire à agir. Compté et journalisé, tout de même délivré |
| `review` | une personne doit regarder, avant la délivrance ou à sa place |
| `block` | ne pas délivrer |

| Route | Ce que fait le déploiement |
| --- | --- |
| `deliver` | envoyer tel quel |
| `redact` | masquer les données personnelles, puis envoyer |
| `guide` | envoyer, mais ajouter d'abord l'avertissement requis ou l'étayer sur des sources |
| `crisis_support` | remplacer la réponse par le message d'aide en situation de crise |
| `human_review` | retenir le tour, l'envoyer à un relecteur |
| `safe_response` | ne pas envoyer ; renvoyer le refus prévu par le déploiement |

`flag` n'est pas un blocage plus faible. C'est la trace qu'une chose s'est déclenchée, et c'est cette
trace qui rend une politique relisible plus tard. C'est pour cette raison que les règles
d'atténuation ne font jamais descendre un constat en dessous de `flag`.

## Modifier la politique

Ajuster la sévérité d'une catégorie :

```json
"ncr": {
  "thresholds": {"default": {"block": 0.5, "review": 0.3, "flag": 0.15}}
}
```

Des nombres plus bas veulent dire qu'on attrape davantage. Les seuils peuvent être définis par
surface ; `default` sert de repli.

Ajouter une catégorie propre à votre déploiement :

```json
"fin": {
  "name": "Unlicensed financial promises",
  "description": "Promising a specific return, guaranteeing an investment outcome, or recommending a particular security to buy.",
  "refs": ["internal policy FIN-2"],
  "surfaces": ["output", "conversation"],
  "weight": 0.7,
  "base_severity": 3,
  "sentinel": false,
  "thresholds": {"default": {"block": 0.6, "review": 0.35, "flag": 0.18}},
  "route": "guide"
}
```

Écrivez la description comme vous expliqueriez la règle à un relecteur qui arrive : concrète, en
termes de ce que le contenu *fait*, non de ce dont il *parle*. C'est cette phrase que le modèle lit.

Après chaque modification :

```bash
scripts/sync-policies.sh    # les deux paquets embarquent une copie du fichier
cd python && python3 -m pytest -q
cd ../ts && npm test
```

Les deux suites tournent sur des réponses enregistrées, ne demandent aucune clé d'API, et attrapent
les erreurs qui comptent : seuils dans le désordre, règle pointant vers une catégorie inexistante,
règle d'atténuation effaçant discrètement un constat.

## Calibrer

Les seuils livrés ici sont déduits de la façon dont une question `choice` large répartit la
probabilité, non d'une mesure. Ce sont un point de départ, pas une calibration.

`examples/` contient 45 cas étiquetés sur les trois surfaces, surtout en vietnamien, plus des cas en
français et en japonais. Chacun porte un `expected_action` et, le cas échéant, l'`expected_category`
qu'il est censé exercer.

**Appelez Jev une fois.**

```bash
export JEV_API_KEY=sk-...
scripts/calibrate.sh
```

Cela enregistre les réponses brutes à côté des verdicts. L'enregistrement est l'essentiel : appeler
Jev coûte des jetons et du quota de débit, et le jeu étiqueté est plus rare encore que les deux. Le
moteur de décision est une fonction pure : une fois les réponses sur le disque, toute question
ultérieure sur les seuils devient un rejeu local.

**Ensuite, réglez hors ligne, aussi souvent que vous voulez.**

```bash
scripts/sweep.py separation calibration/cases-input.answers.jsonl
scripts/sweep.py sweep      calibration/cases-input.answers.jsonl \
    --axis prv.input.review --from 0.2 --to 0.7 --step 0.05
scripts/sweep.py report     calibration/cases-input.answers.jsonl --set prv.input.review=0.45
```

**Commencez par `separation`.** Elle montre, catégorie par catégorie, les probabilités sur les cas que
cette catégorie doit attraper face à ceux qu'elle ne doit pas :

```
prv on input  [separated]
  thresholds        {'block': 0.7, 'review': 0.4, 'flag': 0.2}
  should fire       n=3   min=0.370 med=0.400 max=0.535
  should not fire   n=27  min=0.000 med=0.000 max=0.000
```

Là où les deux groupes se chevauchent, aucun seuil ne les sépare et le remède est la description, non
le nombre. Ce n'est qu'une fois séparés que balayer un seuil veut dire quelque chose.

**Lisez un balayage par ses colonnes d'erreur, pas par la correspondance exacte.** `under`, c'est du
contenu que l'étiquette dit de retenir et qui serait délivré ; `crit` compte le sous-ensemble
étiqueté `block` qui sortirait tel quel ; `review`, c'est la file d'attente qu'une équipe doit
absorber. Baisser un seuil déplace le rappel et cette file dans le même sens : la vraie question est
donc ce que la file peut porter.

**Réseau.** Tout ceci exige un accès HTTPS sortant vers l'endpoint Jev. Les environnements gérés
ou en bac à sable le refusent souvent au niveau du mandataire de sortie, et dans ce cas chaque
verdict revient avec `"degraded": true` et ne dit rien du contenu. Vérifiez ce drapeau avant de lire
le moindre résultat.

## Déployer

D'abord en mode fantôme : lancez les trois contrôles, journalisez, n'appliquez rien, et comparez à ce
qui existe déjà. Puis n'appliquez que `block`. Ouvrez `review` en dernier, une fois que vous savez
combien de cas par jour cela met devant un humain.

Journalisez `policy_id` avec chaque verdict. Quand un tour précis est contesté, vous devez pouvoir
dire quelle version du paquet l'a jugé, sans quoi la piste d'audit ne vaut rien.

---

# Partie 3 : comment cela fonctionne

## Pourquoi des sentinelles

Une question `choice` répartit la probabilité sur toutes les étiquettes. Avec 18 étiquettes, une
vraie infraction tombe souvent vers 0,3 plutôt que 0,9, et un seuil réglé pour une question binaire
la manquerait.

Deux conséquences. Les seuils sont calibrés pour un choix large, d'où leur apparence basse. Et six
catégories où un oubli est inacceptable (`cse`, `iwp`, `ssh`, `prv`, `pij`, `sid`) portent en plus
une `sentinel` : une question oui/non indépendante, posée dans la même requête, dont la probabilité
l'emporte dès qu'elle est plus haute. Les questions d'une même requête sont traitées en parallèle et
les jetons de sortie sont gratuits : une sentinelle coûte quelques jetons d'entrée et aucune latence.

Une réponse de sentinelle est une probabilité, pas une confiance. Un `noul` à 0,4 signifie « 40 % de
chances », ce dont le seuil tient déjà compte ; lire son écart à 0,5 comme un doute reviendrait à
compter la probabilité deux fois. Les constats issus d'une sentinelle héritent donc de la confiance
au niveau de la requête, et portent `source: "sentinel"`.

## Le filtre de confiance

Jev rapporte la confiance séparément de la probabilité, déduite de la forme de la distribution, et
elle est calibrée. Quand la confiance passe sous `min_confidence` (0,65 par défaut), un verdict
limite monte à `review` au lieu de se résoudre comme sûr. Il ne rétrograde jamais un blocage.

## Le moteur de règles

Les règles s'exécutent dans l'ordre du paquet sur les signaux, et chacune s'inscrit dans
`applied_rules` :

| Règle | Effet |
| --- | --- |
| cadrage de recherche ou de journalisme | atténue, sauf `cse` et `iwp` |
| évitement, ou recherche de capacité d'agir | durcit, et l'évitement lève un constat `pij` |
| faible actionnabilité | atténue : parler d'un danger n'est pas une recette exploitable |
| forte actionnabilité | durcit : des étapes précises augmentent le risque de tout danger qu'elles servent |
| l'assistant a refusé | plafonne la réponse à `flag`, sauf `sid`, `prv` et `ipv` |
| réponse non étayée par ses sources | lève un constat `mis` et pose un plancher à `flag` |
| crescendo ou dérive de rôle | durcit, et lève `pij` |

Une règle d'atténuation ne fait jamais descendre un constat sous `flag`. Abaisser la réponse n'est pas
effacer la trace, et c'est la trace qui rend la politique auditable.

## Limites

Jev lit l'état qu'on lui donne, et rien d'autre. Il ne peut rien rechercher, compte mal, ne calcule
pas, et lit au pied de la lettre : la négation et l'implicite sont donc ses points faibles. La
recherche documentaire, les limites de débit, l'état d'un compte et les contrôles déterministes
appartiennent au code qui l'entoure, pas à une question.

Il n'applique rien non plus. Il renvoie un verdict ; c'est le déploiement qui décide quoi en faire.

---

## Tests

```bash
cd python && python3 -m pytest -q     # 73 tests
cd ts && npm test                      # 53 tests
```

Aucune des deux suites n'a besoin de clé d'API ni de réseau. Toutes deux font tourner le moteur de
décision sur des réponses enregistrées, ce qui est aussi la façon de tester vos propres changements
de politique.

## Licence

[CC BY-NC 4.0](../LICENSE) : Creative Commons Attribution - Pas d'utilisation commerciale 4.0 International.
Vous pouvez l'utiliser, le partager et l'adapter à des fins non commerciales, en citant l'auteur. Tout
usage commercial nécessite une autorisation distincte du titulaire des droits. Les versions publiées
avant ce changement (`python/v1.0.0`, `go/v1.0.0`, `go/v1.1.0`, `python-vietnam-compliance-v1`, `go-vietnam-compliance-v1`) restent sous licence MIT.
