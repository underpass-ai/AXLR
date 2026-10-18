#!/usr/bin/env python3
"""Contract clause checklist per contract type (nda, services, lease, employment, sale_of_goods,
software_license, dpa): each clause's purpose and the questions to ask. With a contract text,
each clause is looked for by keyword and heading heuristics in English and Spanish and reported
found (with the matching snippet) or missing. The search is heuristic: a keyword found does not
mean the clause is adequate, and a clause worded differently can be missed. Not legal advice.

Input (JSON on stdin):
  contract_type  "nda" | "services" | "lease" | "employment" | "sale_of_goods" | "software_license" | "dpa"
  text           the contract text, inline (optional)
  language       "en" | "es"   clause titles (default en)
  list_types     true to list the contract types
"""
import json
import re
import sys
import unicodedata

DISCLAIMER = {"en": "Checklist for review, not legal advice: the clauses a contract needs depend on the law that governs it and on the deal.",
              "es": "Lista de comprobación orientativa, no es asesoramiento jurídico: las cláusulas necesarias dependen de la ley aplicable y del negocio."}
HEURISTIC = ("Heuristic keyword and heading search in English and Spanish: 'found' means a matching word was seen, not that the "
             "clause is complete or valid; 'missing' clauses may be worded differently. Read the contract.")

# id: (title en, title es, purpose, questions, keyword patterns on accent-free lower-case text)
C = {
    "parties": ("Parties and recitals", "Partes y exponen", "Identify who is bound, in what capacity, and the background of the deal.",
                ["Are the legal names, registration numbers and addresses correct?", "Who signs, and with what authority?"],
                [r"parties", r"between\b", r"recitals", r"whereas", r"\bpartes\b", r"reunidos", r"intervienen", r"exponen"]),
    "definitions": ("Definitions", "Definiciones", "Fix the meaning of terms used throughout.", ["Are key terms defined once and used consistently?"],
                    [r"definitions", r"defined terms", r"definiciones", r"se entendera por"]),
    "term_termination": ("Term and termination", "Duración y resolución", "Say when the contract starts and ends, how it renews and how either party can end it.",
                         ["Fixed term or indefinite? Automatic renewal?", "Termination for convenience, for breach, notice periods?", "What survives termination?"],
                         [r"\bterm\b", r"terminat", r"duracion", r"vigencia", r"resolucion", r"rescision", r"extincion", r"prorroga"]),
    "fees_payment": ("Price and payment", "Precio y forma de pago", "Set the price, invoicing, payment terms, taxes and late-payment consequences.",
                     ["Fixed price, rates or milestones?", "Payment period and late-payment interest?", "Are taxes (VAT) included or added?"],
                     [r"\bfees?\b", r"\bprice\b", r"payment", r"invoic", r"\bprecio\b", r"\bpago\b", r"factura", r"honorarios", r"retribucion"]),
    "confidentiality": ("Confidentiality", "Confidencialidad", "Protect non-public information exchanged under the contract.",
                        ["What is confidential and for how long?", "Who may receive it (employees, advisers)?"],
                        [r"confidential", r"confidencial", r"deber de secreto", r"\bsecreto\b"]),
    "personal_data": ("Personal data protection", "Protección de datos personales", "Allocate data protection roles and obligations (GDPR) where personal data is processed.",
                      ["Is either party a processor for the other (then a GDPR Art. 28 contract is needed)?", "Are international transfers involved?"],
                      [r"personal data", r"data protection", r"gdpr", r"datos personales", r"proteccion de datos", r"\brgpd\b", r"\blopdgdd\b"]),
    "ip": ("Intellectual property", "Propiedad intelectual e industrial", "Say who owns pre-existing and newly created IP and what licences are granted.",
           ["Who owns deliverables and work product?", "Is pre-existing IP licensed, and on what terms?"],
           [r"intellectual property", r"\bip\b rights", r"copyright", r"propiedad intelectual", r"propiedad industrial", r"derechos de explotacion"]),
    "warranties": ("Representations and warranties", "Manifestaciones y garantías", "Record what each party promises about facts, quality or authority.",
                   ["What is warranted and for how long?", "What is the remedy for breach of warranty?"],
                   [r"warrant", r"represent", r"garantia", r"manifestaciones", r"garantiza"]),
    "liability": ("Limitation of liability", "Limitación de responsabilidad", "Cap and exclude liability, and say what cannot be limited.",
                  ["Is there a cap, and how is it set?", "Are indirect or consequential losses excluded?", "Are fraud, wilful misconduct and death or injury carved out?"],
                  [r"limitation of liability", r"\bliabilit", r"responsabilidad", r"\bliable\b"]),
    "indemnity": ("Indemnities", "Indemnidades", "Allocate the cost of specific third-party claims or losses.",
                  ["Which claims are indemnified, and is there a cap?", "Procedure: notice, control of defence, cooperation?"],
                  [r"indemnif", r"indemn", r"hold harmless", r"mantener indemne"]),
    "force_majeure": ("Force majeure", "Fuerza mayor", "Excuse performance prevented by events beyond the parties' control.",
                      ["Which events count?", "Notice, mitigation and termination after a long event?"],
                      [r"force majeure", r"fuerza mayor", r"caso fortuito"]),
    "assignment": ("Assignment and subcontracting", "Cesión y subcontratación", "Control whether rights and obligations can be transferred or subcontracted.",
                   ["Is consent needed to assign? Change of control?", "May obligations be subcontracted, and who remains liable?"],
                   [r"assign", r"subcontract", r"\bcesion\b", r"ceder", r"subcontrat"]),
    "notices": ("Notices", "Notificaciones", "Say how formal communications must be given.", ["Addresses, e-mail accepted, when is notice received?"],
                [r"notices", r"notification", r"notificaciones", r"comunicaciones entre las partes"]),
    "entire_agreement": ("Entire agreement and amendments", "Acuerdo íntegro y modificaciones", "Exclude prior agreements and require amendments in writing.",
                         ["Do side letters or prior drafts survive?", "How can the contract be changed?"],
                         [r"entire agreement", r"amendment", r"acuerdo integro", r"totalidad del acuerdo", r"modificacion"]),
    "governing_law": ("Governing law and jurisdiction", "Ley aplicable y jurisdicción", "Choose the law that governs and the courts or arbitration that decide disputes.",
                      ["Which law and which courts or arbitral rules?", "Is the choice valid for consumers or employees, who may have mandatory protections?"],
                      [r"governing law", r"governed by", r"jurisdiction", r"arbitrat", r"ley aplicable", r"jurisdiccion", r"fuero", r"juzgados y tribunales", r"arbitraje", r"legislacion aplicable"]),
    "signatures": ("Signatures", "Firmas", "Execute the contract validly.", ["Are all parties signing, with dates and capacity?", "Electronic signature acceptable?"],
                   [r"signature", r"signed", r"in witness whereof", r"firma", r"firmado", r"en prueba de conformidad"]),
    # NDA
    "nda_definition": ("Definition of confidential information", "Definición de información confidencial", "Fix what information is protected, in any form.",
                       ["Is it mutual or one-way?", "Does it cover oral information and information marked later?"],
                       [r"confidential information", r"informacion confidencial"]),
    "nda_exclusions": ("Exclusions", "Exclusiones", "Exclude information that is public, already known, independently developed or lawfully received from third parties.",
                       ["Are the four standard exclusions there?", "Who bears the burden of proving an exclusion?"],
                       [r"exclusion", r"shall not include", r"does not include", r"publicly available", r"public domain", r"exclusiones", r"no se considerara", r"dominio publico"]),
    "nda_purpose_use": ("Permitted purpose and use", "Finalidad y uso permitido", "Limit use of the information to the stated purpose.",
                        ["Is the purpose specific enough?", "Who may receive it (need to know)?"],
                        [r"purpose", r"use solely", r"only for", r"finalidad", r"unicamente para", r"exclusivamente para"]),
    "nda_compelled": ("Compelled disclosure", "Revelación exigida por ley", "Allow disclosure required by law or court order, with notice to the discloser.",
                      ["Is prompt notice required so the discloser can object?"],
                      [r"required by law", r"court order", r"compelled", r"requerimiento", r"orden judicial", r"exigid[ao] por (la )?ley", r"autoridad competente"]),
    "nda_return": ("Return or destruction", "Devolución o destrucción", "Require return or destruction of information at the end or on request.",
                   ["Can archival back-ups be kept?", "Is a certificate of destruction required?"],
                   [r"return", r"destr", r"devolucion", r"devolver", r"destruccion", r"destruir"]),
    "nda_no_licence": ("No licence, no warranty", "Sin licencia ni garantía", "Make clear that disclosure grants no rights and no warranty of accuracy.",
                       ["Is the information provided 'as is'?"], [r"no licen[cs]e", r"as is", r"no rights", r"ninguna licencia", r"no otorga"]),
    "nda_remedies": ("Remedies", "Consecuencias del incumplimiento", "Provide injunctive relief and damages or a penalty for breach.",
                     ["Injunctive relief available? A penalty clause (cláusula penal) and is it enforceable?"],
                     [r"injunct", r"remed", r"damages", r"penalt", r"clausula penal", r"danos y perjuicios", r"incumplimiento"]),
    "nda_duration": ("Duration of the obligation", "Duración de la obligación de confidencialidad", "Say how long the obligation lasts after disclosure or termination.",
                     ["A number of years, or indefinite for trade secrets?"],
                     [r"\byears?\b", r"surviv", r"\banos\b", r"indefinid", r"duracion"]),
    # services
    "scope": ("Scope of services and deliverables", "Objeto y alcance de los servicios", "Describe what will be delivered, to what standard and by when.",
              ["Is there a statement of work?", "How are changes to scope agreed (change control)?"],
              [r"scope", r"services", r"statement of work", r"deliverable", r"\bobjeto\b", r"alcance", r"servicios", r"entregables"]),
    "service_levels": ("Service levels and acceptance", "Niveles de servicio y aceptación", "Set measurable standards, acceptance tests and credits for failures.",
                       ["Are SLAs measurable? Service credits?", "How and when are deliverables accepted?"],
                       [r"service level", r"\bsla\b", r"acceptance", r"niveles? de servicio", r"aceptacion"]),
    "insurance": ("Insurance", "Seguros", "Require cover for the risks the contract allocates.", ["Which policies, minimum amounts, evidence of cover?"],
                  [r"insurance", r"seguro", r"poliza"]),
    "non_solicitation": ("Non-solicitation", "No captación de personal", "Restrict hiring each other's staff during and after the contract.",
                         ["Is the restriction proportionate and lawful where applied?"], [r"solicit", r"no captacion", r"contratar al personal"]),
    # lease
    "premises": ("Description of the property", "Descripción del inmueble", "Identify the property, its annexes (parking, storage) and condition.",
                 ["Land registry and cadastral references?", "Inventory of contents and condition report?"],
                 [r"premises", r"property", r"\binmueble\b", r"vivienda", r"\blocal\b", r"finca", r"referencia catastral"]),
    "permitted_use": ("Permitted use", "Destino o uso", "Fix what the property may be used for.", ["Residential or business use? Is subletting or tourism use excluded?"],
                      [r"permitted use", r"\buse of the\b", r"\bdestino\b", r"\buso\b"]),
    "rent": ("Rent and updates", "Renta y actualización", "Set the rent, payment and any indexation.",
             ["Amount, payment date and method?", "How is rent updated (index, cap under the applicable law)?"],
             [r"\brent\b", r"\brenta\b", r"alquiler", r"actualizacion"]),
    "deposit": ("Deposit and guarantees", "Fianza y garantías", "Secure the tenant's obligations and say how the deposit is returned.",
                ["Legal deposit amount and where it is lodged?", "Additional guarantees (bank guarantee, guarantor)?"],
                [r"deposit", r"\bfianza\b", r"garantia adicional", r"aval"]),
    "expenses": ("Expenses, utilities and taxes", "Gastos, suministros y tributos", "Allocate utilities, community charges and property taxes.",
                 ["Who pays community fees and property tax?", "Are utilities in the tenant's name?"],
                 [r"utilities", r"service charge", r"expenses", r"gastos", r"suministros", r"\bibi\b", r"comunidad"]),
    "repairs": ("Maintenance and repairs", "Conservación y reparaciones", "Allocate repair obligations between landlord and tenant.",
                ["Which repairs are the landlord's and which the tenant's (minor wear and tear)?"],
                [r"repair", r"mainten", r"reparacion", r"conservacion", r"mantenimiento"]),
    "alterations": ("Alterations and works", "Obras", "Control works by the tenant and the landlord.", ["Is consent needed for works? Who keeps improvements?"],
                    [r"alteration", r"\bworks\b", r"\bobras\b", r"mejoras"]),
    "sublet": ("Assignment and subletting", "Cesión y subarriendo", "Control whether the tenant may assign or sublet.", ["Is written consent needed?"],
               [r"sublet", r"sub-?lease", r"assign", r"subarriendo", r"cesion del contrato"]),
    "handover": ("Return of the property", "Devolución del inmueble", "Set the condition on return, inspection and deposit settlement.",
                 ["Inspection on exit? Time to return the deposit?"], [r"surrender", r"vacat", r"\breturn\b", r"devolucion", r"entrega de llaves"]),
    # employment
    "position": ("Position, duties and place of work", "Puesto, funciones y lugar de trabajo", "Describe the job, professional group and where it is performed.",
                 ["Job title and professional classification?", "Remote work arrangements?"],
                 [r"position", r"duties", r"job title", r"place of work", r"puesto", r"funciones", r"grupo profesional", r"centro de trabajo", r"lugar de trabajo"]),
    "contract_duration": ("Start date and type of contract", "Fecha de inicio y modalidad", "State the start date and whether the contract is indefinite or temporary, with its legal cause.",
                          ["If temporary, is the legal cause stated and valid?"],
                          [r"commencement", r"start date", r"fixed[- ]term", r"indefinite", r"fecha de inicio", r"indefinid", r"temporal", r"duracion del contrato"]),
    "probation": ("Probation period", "Período de prueba", "Set a trial period within legal or collective-agreement limits.", ["Length allowed by law or the collective agreement?"],
                  [r"probation", r"trial period", r"periodo de prueba"]),
    "working_time": ("Working time", "Jornada y horario", "Set hours, schedule, overtime and time recording.", ["Full or part time? Distribution of hours? Overtime pay?"],
                     [r"working time", r"working hours", r"hours of work", r"overtime", r"jornada", r"horario", r"horas extra"]),
    "salary": ("Salary and benefits", "Retribución", "Set pay, its components, payment frequency and extra payments.",
               ["Gross annual salary and components?", "Does it meet the collective agreement and minimum wage?"],
               [r"salary", r"remuneration", r"\bwages?\b", r"compensation", r"salario", r"retribucion", r"pagas extra", r"nomina"]),
    "leave": ("Holidays and leave", "Vacaciones y permisos", "Set annual leave and other leave.", ["Days of annual leave? How are dates set?"],
              [r"holiday", r"annual leave", r"vacation", r"vacaciones", r"permisos"]),
    "collective_agreement": ("Applicable collective agreement", "Convenio colectivo aplicable", "Name the collective agreement that governs the relationship.",
                             ["Which agreement, and is the classification consistent with it?"], [r"collective agreement", r"convenio colectivo"]),
    "non_compete": ("Non-competition", "No competencia", "Restrict competition during or after employment, with compensation where required.",
                    ["Is a post-termination restriction limited in time and compensated as the law requires?"],
                    [r"non-?compet", r"no competencia", r"pacto de no concurrencia", r"concurrencia"]),
    "employee_privacy": ("Employee data protection information", "Información sobre protección de datos del trabajador", "Inform the employee about processing of their data (GDPR Art. 13).",
                         ["Is there an Art. 13 notice? Monitoring of devices or video surveillance disclosed?"],
                         [r"personal data", r"data protection", r"datos personales", r"proteccion de datos"]),
    "notice_period": ("Notice of termination", "Preaviso", "Set notice periods for resignation and dismissal.", ["Periods in law and in the collective agreement?"],
                      [r"notice period", r"\bnotice\b", r"preaviso"]),
    # sale of goods
    "goods": ("Goods, quantity and specifications", "Mercancías, cantidad y especificaciones", "Describe the goods precisely.", ["Specifications, quantities, tolerances, packaging?"],
              [r"\bgoods\b", r"products", r"specification", r"quantity", r"mercancia", r"productos", r"especificaciones", r"cantidad"]),
    "delivery": ("Delivery", "Entrega", "Set place, date and terms of delivery (e.g. an Incoterms rule).", ["Which Incoterms rule and version? Late-delivery consequences?"],
                 [r"deliver", r"incoterms", r"shipment", r"entrega", r"envio"]),
    "risk_title": ("Passing of risk and retention of title", "Transmisión del riesgo y reserva de dominio", "Say when risk and ownership pass.",
                   ["When does risk pass? Is title retained until payment?"],
                   [r"\brisk\b", r"\btitle\b", r"retention of title", r"riesgo", r"reserva de dominio", r"propiedad de la mercancia"]),
    "inspection": ("Inspection and notice of defects", "Inspección y denuncia de defectos", "Set the time and form to examine goods and give notice of non-conformity.",
                   ["Time limit to give notice of defects?"], [r"inspect", r"examin", r"non-?conform", r"inspeccion", r"defectos", r"vicios"]),
    "product_warranty": ("Warranty of conformity", "Garantía de conformidad", "Set the warranty period and remedies (repair, replacement, price reduction, termination).",
                         ["Warranty period and remedies? Exclusions?"], [r"warrant", r"conformity", r"garantia", r"conformidad"]),
    "cisg": ("International sale: CISG", "Compraventa internacional: CISG", "Decide whether the UN Convention on Contracts for the International Sale of Goods applies; it applies by default between parties in Contracting States unless excluded (Art. 6).",
             ["Is the CISG excluded or accepted expressly?"], [r"\bcisg\b", r"vienna convention", r"convencion de viena", r"united nations convention on contracts"]),
    # software licence
    "licence_grant": ("Licence grant and scope", "Concesión y alcance de la licencia", "Define what may be done with the software: users, devices, territory, term, exclusivity.",
                      ["Named users, concurrent or enterprise? Affiliates? Production and test use?"],
                      [r"grant", r"licen[cs]e", r"licencia", r"concede", r"otorga"]),
    "restrictions": ("Restrictions", "Restricciones de uso", "List prohibited acts (copying, reverse engineering, sublicensing) within what the law allows.",
                     ["Are restrictions consistent with mandatory exceptions (e.g. decompilation for interoperability)?"],
                     [r"restriction", r"shall not", r"reverse engineer", r"restricciones", r"no podra", r"ingenieria inversa"]),
    "support": ("Support and maintenance", "Soporte y mantenimiento", "Set updates, bug fixing, response times and support fees.", ["Which versions are supported, and for how long?"],
                [r"support", r"maintenance", r"updates", r"soporte", r"mantenimiento", r"actualizaciones"]),
    "ip_indemnity": ("IP infringement indemnity", "Indemnidad por infracción de derechos", "Protect the licensee against third-party IP claims about the software.",
                     ["Remedies: modify, replace, refund? Exclusions for combinations?"], [r"infring", r"indemn", r"infraccion"]),
    "open_source": ("Third-party and open-source components", "Componentes de terceros y de código abierto", "Disclose third-party and open-source components and their licences.",
                    ["Is there a list of components and licences? Copyleft obligations?"], [r"open source", r"third[- ]party (software|components)", r"codigo abierto", r"software libre"]),
    "audit": ("Licence audit", "Auditoría de licencias", "Let the licensor verify compliance.", ["Notice, frequency, cost allocation?"],
              [r"\baudit", r"auditor", r"verificacion del cumplimiento"]),
    "escrow": ("Source code escrow", "Depósito del código fuente", "Protect the licensee if the licensor fails.", ["Release events? Escrow agent?"],
               [r"escrow", r"deposito del codigo", r"codigo fuente"]),
    # dpa (GDPR Art. 28)
    "dpa_subject_duration": ("Subject-matter and duration of the processing", "Objeto y duración del tratamiento", "Art. 28(3) requires the contract to set out the subject-matter and duration of the processing.",
                             ["What processing is covered and until when?"], [r"subject[- ]matter", r"duration", r"\bobjeto\b", r"duracion"]),
    "dpa_nature_purpose": ("Nature and purpose of the processing", "Naturaleza y finalidad del tratamiento", "Art. 28(3): the nature and purpose of the processing.",
                           ["Which operations (collection, storage, hosting) and for what purpose?"], [r"nature", r"purpose", r"naturaleza", r"finalidad"]),
    "dpa_data_subjects": ("Type of personal data and categories of data subjects", "Tipo de datos personales y categorías de interesados", "Art. 28(3): the type of personal data and categories of data subjects.",
                          ["Are special categories (Art. 9) included?"], [r"categories of data subjects", r"types? of personal data", r"categorias de interesados", r"tipo de datos", r"categorias de datos"]),
    "dpa_controller_rights": ("Obligations and rights of the controller", "Obligaciones y derechos del responsable", "Art. 28(3): the obligations and rights of the controller.",
                              ["Are the controller's instructions, audit rights and duties set out?"], [r"obligations and rights of the controller", r"obligaciones y derechos del responsable", r"obligaciones del responsable"]),
    "dpa_a_instructions": ("Processing only on documented instructions", "Tratamiento según instrucciones documentadas", "Art. 28(3)(a): process only on documented instructions from the controller, including on transfers to third countries, unless EU or Member State law requires otherwise (then inform the controller first).",
                           ["Where are the instructions documented?", "Does it cover third-country transfers?"],
                           [r"documented instructions", r"instructions (of|from) the controller", r"instrucciones documentadas", r"instrucciones del responsable"]),
    "dpa_b_confidentiality": ("Confidentiality of authorised persons", "Confidencialidad del personal autorizado", "Art. 28(3)(b): persons authorised to process the data are bound by confidentiality.",
                              ["Are staff and contractors under a written or statutory duty of confidentiality?"], [r"confidential", r"confidencialidad", r"deber de secreto"]),
    "dpa_c_security": ("Security of processing (Art. 32)", "Seguridad del tratamiento (art. 32)", "Art. 28(3)(c): take all measures required by Article 32.",
                       ["Are the technical and organisational measures described, e.g. in an annex?"],
                       [r"article 32", r"art\.? 32", r"security measures", r"technical and organi[sz]ational measures", r"articulo 32", r"medidas de seguridad", r"medidas tecnicas y organizativas"]),
    "dpa_d_subprocessors": ("Sub-processors", "Subencargados del tratamiento", "Art. 28(3)(d), 28(2) and 28(4): engage another processor only with the controller's prior specific or general written authorisation, flow down the same obligations, and remain liable.",
                            ["Specific or general authorisation? How are changes notified and objected to?", "Is there a list of current sub-processors?"],
                            [r"sub-?processor", r"sub-?processing", r"another processor", r"subencargad", r"subcontrat"]),
    "dpa_e_data_subject_rights": ("Assistance with data subject rights", "Asistencia en el ejercicio de derechos", "Art. 28(3)(e): assist the controller, by appropriate technical and organisational measures, in responding to data subject requests (Chapter III).",
                                  ["How fast must requests received by the processor be passed on?"],
                                  [r"data subjects?'? rights", r"rights of (the )?data subjects", r"data subject requests", r"derechos de los interesados", r"ejercicio de (los )?derechos"]),
    "dpa_f_assistance_32_36": ("Assistance with Articles 32 to 36", "Asistencia en las obligaciones de los arts. 32 a 36", "Art. 28(3)(f): assist with security, personal data breach notification and communication, DPIAs and prior consultation.",
                               ["Breach notification deadline to the controller?", "Help with DPIAs?"],
                               [r"articles 32 to 36", r"arts?\.? 32 (to|-) 36", r"articulos 32 a 36", r"breach", r"impact assessment", r"prior consultation", r"violacion de (la )?seguridad", r"brecha", r"evaluacion de impacto", r"consulta previa"]),
    "dpa_g_deletion_return": ("Deletion or return at the end", "Supresión o devolución al finalizar", "Art. 28(3)(g): at the controller's choice, delete or return all personal data at the end of the services and delete copies, unless law requires storage.",
                              ["Who chooses, and within what time? Certificate of deletion?"],
                              [r"delet", r"return (of )?(all )?(the )?personal data", r"destino de los datos", r"devolucion", r"supresion", r"destruccion"]),
    "dpa_h_audits": ("Information and audits", "Información y auditorías", "Art. 28(3)(h): make available all information needed to demonstrate compliance and allow and contribute to audits, including inspections.",
                     ["Audit frequency, notice and costs? Certifications accepted instead?"], [r"\baudit", r"inspection", r"auditori", r"inspeccion"]),
    "dpa_infringing_instructions": ("Notice of infringing instructions", "Aviso de instrucciones contrarias a la normativa", "Art. 28(3), last subparagraph: the processor immediately informs the controller if an instruction infringes data protection law.",
                                    ["Is the duty to warn stated?"], [r"infring", r"contrari[ao] (a|al) (la normativa|el rgpd|reglamento)", r"infrinja", r"vulnera"]),
    "dpa_transfers": ("International transfers", "Transferencias internacionales", "Chapter V GDPR: transfers outside the EEA need an adequacy decision or appropriate safeguards (e.g. standard contractual clauses).",
                      ["Where is the data hosted and accessed from? Which transfer tool?"],
                      [r"transfer", r"third countr", r"standard contractual clauses", r"transferencia", r"terceros paises", r"clausulas contractuales tipo"]),
    "dpa_breach": ("Personal data breach notification", "Notificación de violaciones de seguridad", "Art. 33(2): the processor notifies the controller without undue delay after becoming aware of a breach; the contract usually fixes a deadline and content.",
                   ["How many hours, to whom, with what information?"], [r"breach", r"violacion de (la )?seguridad", r"brecha"]),
}
DPA_REFS = {"dpa_subject_duration": "GDPR Art. 28(3)", "dpa_nature_purpose": "GDPR Art. 28(3)", "dpa_data_subjects": "GDPR Art. 28(3)",
            "dpa_controller_rights": "GDPR Art. 28(3)", "dpa_a_instructions": "GDPR Art. 28(3)(a)", "dpa_b_confidentiality": "GDPR Art. 28(3)(b)",
            "dpa_c_security": "GDPR Art. 28(3)(c)", "dpa_d_subprocessors": "GDPR Art. 28(3)(d)", "dpa_e_data_subject_rights": "GDPR Art. 28(3)(e)",
            "dpa_f_assistance_32_36": "GDPR Art. 28(3)(f)", "dpa_g_deletion_return": "GDPR Art. 28(3)(g)", "dpa_h_audits": "GDPR Art. 28(3)(h)",
            "dpa_infringing_instructions": "GDPR Art. 28(3), second subparagraph", "dpa_transfers": "GDPR Chapter V (Arts. 44-49)", "dpa_breach": "GDPR Art. 33(2)"}
MANDATORY_DPA = {k for k, v in DPA_REFS.items() if v.startswith("GDPR Art. 28")}
TYPES = {
    "nda": ("Non-disclosure agreement", ["parties", "nda_definition", "nda_exclusions", "nda_purpose_use", "confidentiality", "nda_compelled", "nda_duration",
                                         "nda_return", "nda_no_licence", "nda_remedies", "personal_data", "assignment", "entire_agreement", "governing_law", "signatures"]),
    "services": ("Services agreement", ["parties", "definitions", "scope", "service_levels", "fees_payment", "term_termination", "ip", "confidentiality",
                                        "personal_data", "warranties", "liability", "indemnity", "insurance", "assignment", "non_solicitation", "force_majeure",
                                        "notices", "entire_agreement", "governing_law", "signatures"]),
    "lease": ("Lease", ["parties", "premises", "permitted_use", "term_termination", "rent", "deposit", "expenses", "repairs", "alterations", "sublet",
                        "insurance", "handover", "personal_data", "notices", "governing_law", "signatures"]),
    "employment": ("Employment contract", ["parties", "position", "contract_duration", "probation", "working_time", "salary", "leave", "collective_agreement",
                                           "confidentiality", "ip", "non_compete", "employee_privacy", "notice_period", "governing_law", "signatures"]),
    "sale_of_goods": ("Sale of goods", ["parties", "goods", "fees_payment", "delivery", "risk_title", "inspection", "product_warranty", "liability",
                                        "force_majeure", "term_termination", "cisg", "governing_law", "entire_agreement", "signatures"]),
    "software_license": ("Software licence", ["parties", "definitions", "licence_grant", "restrictions", "fees_payment", "support", "ip", "warranties",
                                              "liability", "ip_indemnity", "open_source", "audit", "escrow", "confidentiality", "personal_data",
                                              "term_termination", "governing_law", "signatures"]),
    "dpa": ("Data processing agreement (GDPR Art. 28)", ["parties", "dpa_subject_duration", "dpa_nature_purpose", "dpa_data_subjects", "dpa_controller_rights",
                                                         "dpa_a_instructions", "dpa_b_confidentiality", "dpa_c_security", "dpa_d_subprocessors",
                                                         "dpa_e_data_subject_rights", "dpa_f_assistance_32_36", "dpa_g_deletion_return", "dpa_h_audits",
                                                         "dpa_infringing_instructions", "dpa_transfers", "dpa_breach", "liability", "governing_law", "signatures"]),
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fold(text):
    """Lower-case and strip accents character by character, keeping positions aligned with the original."""
    out = []
    for ch in text:
        base = unicodedata.normalize("NFD", ch)[0]
        low = base.lower()
        out.append(low if len(low) == 1 else base)
    return "".join(out)


def word_end(text, i):
    while i < len(text) and text[i].isalpha():
        i += 1
    return i


def snippet(text, start, end, width=200):
    left = max(text.rfind("\n", 0, start), text.rfind(". ", 0, start))
    left = left + 1 if left >= 0 else 0
    if start - left > 80:
        left = start - 80
    ends = [i for i in (text.find("\n", end), text.find(". ", end)) if i >= 0]
    right = min(ends) + 1 if ends else len(text)
    if right - left > width:
        right = left + width
    return " ".join(text[left:right].split())


def search(text, folded, patterns):
    best = None
    for pattern in patterns:
        m = re.search(r"(?<![a-z0-9])" + pattern, folded)
        if m and (best is None or m.start() < best[0].start()):
            best = (m, pattern)
    if best is None:
        return None
    m, _ = best
    return {"matched": text[m.start():word_end(folded, m.end())], "snippet": snippet(text, m.start(), m.end())}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if request.get("list_types"):
        print(json.dumps({"contract_types": {k: {"title": v[0], "clauses": len(v[1])} for k, v in TYPES.items()}}))
        return
    ctype = request.get("contract_type")
    if ctype not in TYPES:
        fail("contract_type must be one of " + ", ".join(TYPES))
    lang = request.get("language", "en")
    if lang not in ("en", "es"):
        fail("language must be en or es")
    text = request.get("text")
    if text is not None and (not isinstance(text, str) or not text.strip()):
        fail("text must be the contract text as a non-empty string")
    if text is not None and len(text) > 2000000:
        fail("text is longer than 2,000,000 characters")
    folded = fold(text) if text else None
    title, ids = TYPES[ctype]
    clauses, found, missing = [], [], []
    for cid in ids:
        en, es, purpose, questions, patterns = C[cid]
        row = {"id": cid, "title": es if lang == "es" else en, "purpose": purpose, "questions": questions}
        if cid in DPA_REFS:
            row["legal_reference"] = DPA_REFS[cid]
            row["mandatory"] = cid in MANDATORY_DPA
        if text:
            hit = search(text, folded, patterns)
            row["status"] = "found" if hit else "missing"
            if hit:
                row.update(hit)
                found.append(cid)
            else:
                missing.append(cid)
        clauses.append(row)
    out = {"contract_type": ctype, "title": title, "language": lang, "clauses": clauses}
    if ctype == "dpa":
        out["mandatory_contents"] = ["28(3)(a)", "28(3)(b)", "28(3)(c)", "28(3)(d)", "28(3)(e)", "28(3)(f)", "28(3)(g)", "28(3)(h)"]
        out["mandatory_note"] = ("GDPR Art. 28(3) requires a contract or legal act setting out the subject-matter, duration, nature and purpose of the "
                                 "processing, the type of personal data, the categories of data subjects and the controller's obligations and rights, "
                                 "and stipulating points (a) to (h); standard contractual clauses under Art. 28(7)-(8) may be used.")
    if text:
        out["summary"] = {"found": len(found), "missing": len(missing), "total": len(ids), "missing_ids": missing}
        if ctype == "dpa":
            out["summary"]["mandatory_missing"] = [c for c in missing if c in MANDATORY_DPA]
        out["heuristic_note"] = HEURISTIC
    out["disclaimer"] = DISCLAIMER[lang]
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
